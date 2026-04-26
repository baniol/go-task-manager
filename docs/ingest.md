# Inbox Ingest — Automatic Task Capture from Slack and Gmail

A global mechanism for pulling actionable items from Slack and Gmail into `tm` as draft tasks for review. Built on top of Claude Code's interactive session — no separate service, no paid API calls beyond your existing Claude Code subscription.

## How it works

When you run `/ingest` in any Claude Code session, the skill:

1. Reads your source configuration (`~/.config/ingest/sources.json`) and the cursor from the last run (`~/.config/ingest/state.json`).
2. Uses the Slack and Gmail MCP connectors (authenticated via claude.ai) to fetch new messages since the last cursor.
3. Classifies each message as **actionable** or **noise** using LLM judgment.
4. Routes actionable items to the configured sinks — by default `tm add` as drafts.
5. Updates the cursor so the next run only processes new messages.

```
/ingest
    │
    ├── Slack MCP  ──► new channel messages + mentions + DMs
    ├── Gmail MCP  ──► new inbox messages matching query
    │
    ├── classify: actionable? ──► tm add --tag ingest,slack (draft)
    │             noise?      ──► skip
    │
    └── write ~/.config/ingest/state.json  (updated cursors)
```

`tm` is just the default sink. The mechanism is source/sink agnostic — see [Sinks](#sinks).

## Setup

### 1. Create the skill file

```bash
cat > ~/.claude/commands/ingest.md << 'EOF'
<contents of the skill — see below>
EOF
```

The skill file lives at `~/.claude/commands/ingest.md`. Once created it is available as `/ingest` in every Claude Code session.

### 2. Configure sources

Edit `~/.config/ingest/sources.json`:

```json
{
  "slack": {
    "channels": [
      { "id": "C0XXXXXXX", "name": "devops" },
      { "id": "D04XXXXXXX", "name": "dm-tomek" }
    ],
    "include_mentions": true,
    "include_dms": true
  },
  "gmail": {
    "query": "in:inbox newer_than:2d",
    "llm_filter": true
  }
}
```

Leave `channels` empty (`[]`) to skip channel scanning and only process mentions and DMs.

### 3. Authenticate Slack and Gmail (first run only)

On the first `/ingest` run, Claude will trigger the OAuth flow for any unauthenticated source. Follow the prompts in the session — this is a one-time step per source.

### 4. Run

Open Claude Code in any directory and type:

```
/ingest
```

Tasks land in `tm` as **drafts** — they do not appear in `tm list` until you promote them:

```bash
tm list --tag ingest --draft   # review candidates
tm publish <id>                 # promote a draft to todo
tm rm <id>                      # discard noise
```

## Sinks

Edit `~/.config/ingest/sinks.json` to configure where classified items go.

### Default — tm draft

```json
{
  "id": "tm_draft",
  "type": "shell",
  "enabled": true,
  "command_template": "tm add {title} --body {body} --tag ingest,{source_tag}"
}
```

### Daily markdown note

```json
{
  "id": "daily_note",
  "type": "markdown_append",
  "enabled": true,
  "path": "~/notes/{date}.md",
  "format": "\n## [{source}] {time}\n**{title}**\n\n{body}\n\n---\n"
}
```

Multiple sinks can be enabled simultaneously — an item can be both a task and a note.

## Deduplication

Each task body ends with a `Source:` line:

```
Source: slack://C0XXXXXXX/1713790123.456789
Source: gmail://18f3abc42d1e5c2a
```

Before adding a task, the skill checks:
1. Whether the source ID is already in the state cursor.
2. Whether any existing `tm` task body contains the same `Source:` line (`tm export | grep`).

Running `/ingest` twice in a row adds zero duplicate tasks.

## Classification criteria

**Actionable** → task created:
- Direct request or question addressed to you or your team
- Incident, outage, or alert requiring a response
- Decision or commitment you need to follow up on
- Explicit TODO assigned to you
- Something a person is waiting on you for

**Noise** → silently skipped:
- Automated notifications with no required action
- FYI / announcement threads not involving you
- Conversations between others
- Already-resolved or already-responded threads
- Marketing, spam

## Upgrading to scheduled runs

Once you trust the classification quality, add a launchd job that opens a headless Claude Code session on a schedule. This requires configuring local MCP servers (Slack bot token + Gmail API credentials) instead of the cloud connectors used in interactive mode — the skill prompt and config files remain unchanged.

See `~/.config/ingest/` for all configuration files.
