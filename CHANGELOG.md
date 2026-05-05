# Changelog

All notable changes to this project will be documented in this file.
Format: [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

### Added
- Done tab in the TUI is sorted by completion time (most recently done first); a new `done_at` column tracks when a task last entered the done state, preserved across sync via export/import.
- TUI logs slow `Update`/`View` frames (>3ms) when run with `--verbose`, with a per-stage breakdown for the detail view, so future input-lag regressions can be diagnosed without re-adding ad-hoc timing.

## [v0.1.4] - 2026-05-04

## [v0.1.3] - 2026-05-04

## [v0.1.2] - 2026-04-26

## [v0.1.1] - 2026-04-22

### Changed
- Task body in detail view is now rendered as Markdown (via glamour)
- `b` key opens `$EDITOR` (fallback: `vi`) instead of the built-in text editor; vim-style `:cq` cancels without saving

## [v0.1.0] - 2026-04-21

### Added
- Task management: add, edit, move between statuses (todo/doing/done), delete
- Priority levels: low, normal, high
- Tags: attach multiple tags per task, filter by tag
- Ready flag: mark tasks as ready, filter by `--ready`
- Worklog: log time entries per task, summarize by task or day
- SQLite storage at `~/.tm/tasks.db` with automatic migrations
- `publish` command to mark tasks ready for review
- Short and long list formats
