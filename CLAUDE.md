# tm — CLI task manager

Go CLI for managing tasks, backed by a local SQLite database at `~/.tm/tasks.db`.

## Commands

```
just build              # vet + build ./tm binary
just test               # go test ./...
just test-race          # tests with the race detector
just smoke              # E2E against an isolated DB in /tmp
just fmt                # gofmt
just changelog-context  # git log since last tag (for writing CHANGELOG)
just release patch|minor|major  # bump version, promote CHANGELOG, commit+tag (no push)
```

## Architecture

```
cmd/tm/main.go               → entry point, calls cli.Execute()
internal/task/task.go        → domain: Task, Status, Priority (pure types, no deps)
internal/store/store.go      → Store interface + input/filter types
internal/store/sqlite.go     → SQLite impl (modernc.org/sqlite, no CGO)
internal/store/migrations/   → embedded SQL migrations (PRAGMA user_version)
internal/cli/                → cobra commands; App struct holds Store
```

Flow: CLI → Store interface → SQLite. Add new commands as methods on `App`
and register them in `root.go:NewRootCmd`.

## Conventions

- CLI helpers: `parseID()`, `printTask()`, `formatTaskShort()` — in root.go and add.go
- Store helpers: `ensureRow()`, `listNames()`, `dedupeStrings()` — avoid duplication
- Any DB operation touching more than one row lives in a transaction
- Input validation at the store boundary (empty titles, etc.)
- Store tests: `newTestStore(t)` with a temp DB; CLI tests: `newHarness(t)` in helpers_test.go
- **All code, comments, docs and identifiers are in English.**
- Commits without `Co-Authored-By` Anthropic

## Release flow

1. Run `just changelog-context` to get commits since last tag
2. Edit `## [Unreleased]` in `CHANGELOG.md` with notable changes — **never write a version header directly**, the script promotes `[Unreleased]` automatically
3. Run `just release patch|minor|major` — promotes the section, commits, tags
4. Review the commit and tag, then `git push --follow-tags`
5. GitHub Actions builds binaries and creates a GitHub Release using the CHANGELOG section
