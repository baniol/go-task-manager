# tm — task manager: handy dev commands
# run `just -l` for the list

set shell := ["bash", "-euo", "pipefail", "-c"]

default: build

# version from git describe (fallback: dev); dirty trees get a -dirty suffix.
version := `git describe --tags --always --dirty 2>/dev/null || echo dev`
ldflags := "-s -w -X 'go-task-manager/internal/cli.version=" + version + "'"

# build the tm binary in the project root (with go vet)
build:
    go vet ./...
    go build -ldflags="{{ldflags}}" -o tm ./cmd/tm

# install to $GOBIN (default ~/go/bin)
install:
    go vet ./...
    go install -ldflags="{{ldflags}}" ./cmd/tm

# run a command through `go run` (e.g. `just run list`)
run *ARGS:
    go run ./cmd/tm {{ARGS}}

# static analysis
vet:
    go vet ./...

# code formatting
fmt:
    gofmt -w -s .

# tests
test:
    go test ./...

# tests with the race detector
test-race:
    go test -race ./...

# tidy go.mod / go.sum
tidy:
    go mod tidy

# remove the built binary from the project dir
clean:
    rm -f tm

# smoke test on an isolated DB in /tmp (doesn't touch ~/.tm)
smoke: build
    #!/usr/bin/env bash
    set -euo pipefail
    tmp=$(mktemp -d)
    trap 'rm -rf "$tmp"' EXIT
    export HOME="$tmp"
    ./tm add write README
    ./tm add --prio high --tag backend --tag api build kanban
    ./tm add --prio low --ready clean up
    ./tm move 1 doing
    ./tm publish 2
    ./tm edit 1 --body "including a Roadmap section"
    ./tm list
    ./tm list --tag backend
    ./tm list --ready
    ./tm tags
    ./tm log add 1 --start "2026-04-17 09:00" --end "2026-04-17 10:30" --note "draft readme"
    ./tm log add 2 --start "2026-04-17 11:00" --end "2026-04-17 12:15" --note "kanban layout"
    ./tm worklog
    ./tm worklog --task 1
    ./tm worklog summary --group-by task

# print git log since last tag — paste into CHANGELOG.md [Unreleased] before releasing
changelog-context:
    #!/usr/bin/env bash
    set -euo pipefail
    since=$(git describe --tags --abbrev=0 2>/dev/null || git rev-list --max-parents=0 HEAD)
    echo "Commits since ${since}:"
    git log "${since}..HEAD" --oneline

# bump semver tag, promote [Unreleased] in CHANGELOG.md, commit and tag (does NOT push)
# usage: just release patch|minor|major
release segment:
    #!/usr/bin/env bash
    set -euo pipefail
    current=$(git describe --tags --abbrev=0 2>/dev/null || echo "v0.0.0")
    version="${current#v}"
    IFS='.' read -r major minor patch <<< "$version"
    case "{{segment}}" in
        major) major=$((major+1)); minor=0; patch=0 ;;
        minor) minor=$((minor+1)); patch=0 ;;
        patch) patch=$((patch+1)) ;;
        *) echo "error: usage: just release patch|minor|major"; exit 1 ;;
    esac
    new="v${major}.${minor}.${patch}"
    today=$(date +%Y-%m-%d)
    if ! grep -q "^## \[Unreleased\]" CHANGELOG.md; then
        echo "error: CHANGELOG.md is missing an [Unreleased] section"; exit 1
    fi
    awk -v ver="${new}" -v date="${today}" '
        /^## \[Unreleased\]/ {
            print "## [Unreleased]"
            print ""
            print "## [" ver "] - " date
            next
        }
        { print }
    ' CHANGELOG.md > CHANGELOG.md.tmp && mv CHANGELOG.md.tmp CHANGELOG.md
    git add CHANGELOG.md
    git commit -m "chore: release ${new}"
    git tag "${new}"
    echo ""
    echo "Tagged ${new}. Review, then push with:"
    echo "  git push --follow-tags"

# clear the gopls cache (when LSP diagnostics go stale)
clean-gopls:
    rm -rf "${HOME}/Library/Caches/gopls"
    @echo "cache cleared — restart your editor / LSP"

# full reset: binary + gopls cache + local .tm database (WARNING: deletes tasks)
nuke:
    rm -f tm
    rm -rf "${HOME}/Library/Caches/gopls"
    rm -rf "${HOME}/.tm"
    @echo "binary, gopls cache and ~/.tm removed"
