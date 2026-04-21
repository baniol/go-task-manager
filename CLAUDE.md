# tm — CLI task manager

Go CLI for managing tasks, backed by a local SQLite database at `~/.tm/tasks.db`.

## Commands

```
just build          # vet + build ./tm binary
just test           # go test ./...
just test-race      # tests with the race detector
just smoke          # E2E against an isolated DB in /tmp
just fmt            # gofmt
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
