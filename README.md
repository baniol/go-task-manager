# tm — CLI task manager

A small educational Go project: a CLI task manager backed by a local SQLite
database.

## Requirements

- Go 1.26+
- No CGO — uses the pure-Go `modernc.org/sqlite` driver.

## Build

```sh
# quick local build (produces ./tm)
go build -o tm ./cmd/tm

# install to $GOBIN (default ~/go/bin)
go install ./cmd/tm
```

## Run

Without installing:

```sh
go run ./cmd/tm <command> [args...]
```

After building:

```sh
./tm <command> [args...]
```

The database lives at `~/.tm/tasks.db` (created on first run).

## Commands

```sh
# tasks
tm add [--prio low|normal|high] [--body ...] [--tag t]... [--ready]
       [--due today|tomorrow|+Nd|YYYY-MM-DD] <title...>
tm list [--status ...] [--prio ...] [--tag t]... [--draft|--ready]
        [--overdue|--no-due] [--sort default|due]
tm move <id> <status>                       # todo | doing | action | done
tm edit <id> [--title ...] [--body ...] [--ready | --draft]
       [--due ...|--clear-due]
tm publish <id>                             # flip draft → ready
tm rm <id>                                  # delete task

# tags (per-task)
tm tag add <id> <tag>...                    # attach tags to a task
tm tag rm  <id> <tag>...                    # detach tags from a task

# tags (global)
tm tag list                                 # list all tags
tm tag delete <name>                        # remove tag system-wide

# full-text search (FTS5, prefix matching)
tm search <query> [--status ...] [--tag t]...

# time tracking
tm start <id> [--note ...]                  # start a timer
tm stop [--at HH:MM|-20m|...]               # stop the active timer (optionally backdate)
tm status                                   # show active timer
tm log <task-id>                            # time entries for a task
tm log add <task-id> --start <t> --end <t> [--note ...]  # manual backfill
tm log edit <entry-id> [--start ...] [--end ...] [--note ...]
tm log rm <entry-id>                        # delete a time entry

# worklog (cross-task)
tm worklog [--from <t>] [--to <t>] [--task <id>] [--tag t]... [--search q] [--limit n]
tm worklog summary --group-by=day|task|tag [same filters]

# interactive TUI (bubbletea)
tm ui

# maintenance
tm backup                                   # snapshot DB to ~/.tm/tasks.db.<timestamp>
tm export > dump.json                       # export full DB as JSON
tm import dump.json                         # restore DB from JSON (replaces)
tm reset                                    # wipe all data

tm help                                     # help
```

New tasks are **drafts** by default (rendered with `~`). Pass `--ready` on
`add` or run `tm publish <id>` to promote them. Tags are created lazily on
first use — no need to register them separately.

Aliases: `ls` = `list`, `mv` = `move`, `delete` = `rm`,
`tags` = `tag`, `tag ls` = `tag list`.

The CLI is built on [`spf13/cobra`](https://github.com/spf13/cobra) — run
`tm <command> --help` for per-command help, and
`tm completion <bash|zsh|fish|powershell>` to generate shell completions.

### Example — tasks

```sh
$ tm add write README
added #1 [draft|normal] write README

$ tm add --prio high --tag backend --tag api build kanban UI
added #2 [draft|high] build kanban UI +api +backend

$ tm publish 2
task #2 → ready

$ tm move 1 doing
task #1 → doing

$ tm edit 1 --title "write README v2" --ready
task #1 [normal] write README v2

$ tm list
ID  STATUS  PRIO    TITLE             TAGS         DUE  CREATED
2   todo    high    build kanban UI   api,backend  -    2026-04-15 20:36
1   doing   normal  write README v2   -            -    2026-04-15 20:36
```

### Example — due dates

```sh
$ tm add --due today "call the dentist"
added #1 [draft|normal] call the dentist @2026-04-16

$ tm add --due +3d --prio high "ship v0.3"
added #2 [draft|high] ship v0.3 @2026-04-19

$ tm list --overdue
ID  STATUS  PRIO    TITLE             TAGS  DUE          CREATED
3   todo    normal  ~ old deadline    -     !2026-01-01  2026-04-16 09:36

$ tm list --sort due
ID  STATUS  PRIO    TITLE                TAGS  DUE          CREATED
3   todo    normal  ~ old deadline       -     !2026-01-01  2026-04-16 09:36
1   todo    normal  ~ call the dentist   -     2026-04-16   2026-04-16 09:36
2   todo    high    ~ ship v0.3          -     2026-04-19   2026-04-16 09:36

$ tm edit 2 --clear-due
task #2 [draft|high] ship v0.3
```

`--due` is interpreted as end-of-day in the local timezone (stored in UTC),
so a task "due today" doesn't become overdue the instant it's created —
overdue kicks in after midnight. `done` tasks are excluded from `--overdue`.

### Example — tags

```sh
# attach tags to a task (many at once)
$ tm tag add 1 docs readme
task #1 [normal] write README v2 +docs +readme

# detach a tag from a task (the tag stays in the system)
$ tm tag rm 1 readme
task #1 [normal] write README v2 +docs

# list all tags
$ tm tag list
api
backend
docs

# remove a tag system-wide
$ tm tag delete api
deleted tag "api", unlinked from 1 task(s)
```

### Example — search

```sh
$ tm search kanban
ID  STATUS  PRIO  TITLE              TAGS         DUE  CREATED
2   todo    high  build kanban UI    api,backend  -    2026-04-15 20:36

# prefix matching — "bac" matches "backend"
$ tm search bac --status todo
ID  STATUS  PRIO  TITLE              TAGS         DUE  CREATED
2   todo    high  build kanban UI    api,backend  -    2026-04-15 20:36
```

Search runs over titles and bodies (FTS5 with prefix matching). Combine
with `--status` and `--tag` filters.

### Example — time tracking

```sh
# start a timer
$ tm start 1 --note "refactor auth"
started timer on #1

$ tm status
#1 0h12m — write README

# stop (optionally backdated, e.g. "I forgot to stop")
$ tm stop --at -5m
stopped timer on #1 (elapsed 0h47m)

# manual backfill (e.g. a forgotten session)
$ tm log add 1 --start "2026-04-17 09:00" --end "2026-04-17 10:30" --note "doc review"
added entry #2 on #1 (1h30m)

# per-task entries
$ tm log 1
#1    2026-04-17 09:13 → 10:00  0h47m  refactor auth
#2    2026-04-17 09:00 → 10:30  1h30m  doc review
total: 2h17m
```

### Example — worklog

Cross-task view: listing and aggregates with range/tag filters.

```sh
# default range is the current ISO week (Mon–Sun)
$ tm worklog
ID  TASK  TITLE         DATE        START  END    DURATION  NOTE
2   #1    write README  2026-04-17  09:00  10:30  1h30m     doc review
1   #1    write README  2026-04-17  09:13  10:00  0h47m     refactor auth
total: 2h17m (2 entries)

# aggregates by task (also --group-by=day|tag)
$ tm worklog summary --group-by=task
KEY  LABEL         COUNT  DURATION
1    write README  2      2h17m
total: 2h17m (2 entries)

# filter by note
$ tm worklog --search auth

# custom range
$ tm worklog --from 2026-04-01 --to 2026-05-01
```

`--from` / `--to` accept `YYYY-MM-DD`, `HH:MM`, `YYYY-MM-DD HH:MM`, RFC3339,
or relatives like `-1h30m`.

### Interactive TUI

```sh
tm ui
```

Launches an interactive task list with three tabs, cycled with
`tab` / `←` / `→`:

- **active** — `todo` + `doing` + `action` tasks,
- **done** — `done` tasks,
- **worklog** — cross-task time entries for the current ISO week (customizable).

Key bindings:

| Key              | Action                               |
|------------------|--------------------------------------|
| `↑`/`k`, `↓`/`j` | move up/down                         |
| `K`/`J`          | reorder task up/down                 |
| `tab` / `←`/`→`  | switch tab                           |
| `enter`          | task details (with time log)         |
| `t`              | cycle status (todo→doing→done→todo)  |
| `1`/`2`/`3`      | priority high/normal/low             |
| `a`              | add a new task                       |
| `e`              | edit title                           |
| `b`              | edit body (ctrl+s saves)             |
| `+` / `-`        | add/remove tags (e.g. `foo bar,baz`) |
| `T`              | filter by tag                        |
| `C`              | pick a context (tag) from a picker   |
| `s`              | start/stop timer on current task     |
| `d`              | delete task, or (on worklog tab) set a custom date range |
| `p`              | publish a draft                      |
| `/`              | live search                          |
| `esc`            | clear filter/search                  |
| `q`              | quit                                 |

In the details view (`enter`), `tab` switches focus between metadata and
the time log; there, `e` edits an entry step-by-step (start → end → note)
and `d` deletes it.

Manual task order (K/J) is persisted to the database and survives restarts.

## Releases

Pre-built binaries for Linux and macOS (arm64/amd64) are attached to each
[GitHub Release](../../releases). Download, make executable, and move to your
`$PATH`:

```sh
curl -L https://github.com/<owner>/go-task-manager/releases/latest/download/tm-<version>-linux-amd64.tar.gz | tar xz
chmod +x tm-*-linux-amd64 && mv tm-*-linux-amd64 ~/bin/tm
```

To cut a release (maintainers):

```sh
just changelog-context          # review commits since last tag
# edit CHANGELOG.md [Unreleased] section
just release minor              # bumps version, commits, tags
git push --follow-tags          # triggers CI build + GitHub Release
```

## Project layout

```
cmd/tm/              # entrypoint (thin — calls cli.Execute)
internal/cli/        # cobra commands, one per file
internal/tui/        # interactive TUI (bubbletea)
internal/task/       # domain model (Task, Status, Priority)
internal/store/      # Store interface + SQLite impl (tag m:n, FTS5)
internal/store/migrations/   # embedded SQL migrations
```
