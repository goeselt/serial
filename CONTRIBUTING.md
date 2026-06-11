# Contributing to Serial

## Design

| File                          | Responsibility                                                                                           |
| ----------------------------- | -------------------------------------------------------------------------------------------------------- |
| `main.go`                     | CLI entry point: flag parsing, NDJSON output (series --> warnings --> duplicates --> summary), progress. |
| `internal/catalog/model.go`   | Data model: Series, Season, Episode, ParseWarning, DuplicateGroup; SortLibrary for stable output.        |
| `internal/scanner/scanner.go` | Filesystem walk: file type filtering, non-portable name checks, episode dispatch, duplicate detect.      |
| `internal/parser/parser.go`   | Filename parser: SxE, NxNN, season/episode path, specials; left-to-right precedence, range expand.       |

The scanner is the only caller of the parser. Duplicate detection runs after the full walk so all paths for an episode
are visible before grouping. The parser is stateless and has no dependency on the scanner or catalog.

## Development Setup

Go 1.24 or later. No external dependencies.

```bash
make build
```

## Local Verification

Run the same checks used by CI:

```bash
make check
```

Lint:

```bash
docker pull ghcr.io/goeselt/pedant:latest
docker run --rm -v "$(pwd):/work" ghcr.io/goeselt/pedant:latest
```

## Submitting Changes

Commit messages and PR titles must follow [Conventional Commits](https://www.conventionalcommits.org/). The release
pipeline uses the PR title to determine the next version.
