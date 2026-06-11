# serial

A command-line tool that scans a local directory for TV series and episodes. It parses season and episode numbers from
file and folder names, groups them into a catalog, and flags duplicates and non-portable filenames.

Output is human-readable on stderr and machine-readable NDJSON on stdout, so it works equally well at the terminal and
as the first stage of a larger pipeline -- even for libraries with thousands of episodes.

## Installation

### Download a Release Binary

Grab the latest binary for your platform from the [Releases](https://github.com/goeselt/serial/releases) page and put it
on your `PATH`.

### Build From Source

```bash
git clone https://github.com/goeselt/serial.git
cd serial
go build -o serial .
```

Requires Go 1.24 or later. No external dependencies.

## Usage

```bash
serial [options] <path>
```

`<path>` is the directory to scan (required). The series name is taken from the top-level folder under it.

### Options

| Flag | Description                                                          |
| ---- | -------------------------------------------------------------------- |
| `-q` | Suppress all stderr output (progress, findings, and summary)         |
| `-n` | Suppress JSON output on stdout; stderr still shows progress/findings |

### Exit Codes

| Code | Meaning                            |
| ---- | ---------------------------------- |
| `0`  | Scan completed                     |
| `1`  | The scan or output encoding failed |
| `2`  | Invalid command-line arguments     |

## Examples

```bash
# Scan a library: progress and findings on stderr, NDJSON on stdout
serial /mnt/media

# Quiet: only NDJSON, no stderr noise
serial -q /mnt/media | jq .

# Findings only: human-readable duplicates and warnings, no JSON
serial -n /mnt/media

# Save the catalog and the log separately
serial /mnt/media > catalog.ndjson 2> scan.log

# List series names
serial -q /mnt/media | jq -r 'select(.type=="series") | .name'

# Show only duplicates
serial -q /mnt/media | jq 'select(.type=="duplicate")'

# Read just the summary without buffering the whole stream
serial -q /mnt/media | tail -1 | jq .
```

## Output

Human-readable progress, findings, and a final summary are written to **stderr**:

```text
serial: scanning /mnt/media
serial: warning non_portable_name: Some: Show: name contains reserved character ":"
serial: duplicate: Baking Bad S01E02 -- Baking Bad/Season 1/ep.avi, Baking Bad/Season 1/ep.mkv
serial: done -- 2 series, 4 episodes, 1 duplicates, 0 parse warnings, 1 scan warnings (8ms)
```

NDJSON is written to **stdout** -- one object per line, each with a `type` field. Records appear in order: every
`series`, then any `warning`, then any `duplicate`, then a single trailing `summary`.

A `series` record carries its seasons and episodes. Per-file parsing problems are attached as `parse_warnings`. A
multi-episode file appears once per episode it covers, with the episodes sharing the same `path`.

```json
{
  "type": "series",
  "name": "Baking Bad",
  "path": "/mnt/media/Baking Bad",
  "seasons": [
    {
      "number": 1,
      "episodes": [
        { "number": 1, "title": "Butter", "path": "Baking Bad/Season 1/Baking.Bad.S01E01.Butter.mkv" },
        { "number": 2, "path": "Baking Bad/Season 1/Baking.Bad.S01E02.mkv" }
      ]
    }
  ]
}
```

The remaining record types:

```json
{ "type": "warning", "code": "non_portable_name", "message": "name contains reserved character \":\"", "path": "Some: Show" }
{ "type": "duplicate", "series_name": "Baking Bad", "season_number": 1, "episode_number": 2, "paths": ["Baking Bad/Season 1/ep.avi", "Baking Bad/Season 1/ep.mkv"] }
{ "type": "summary", "series": 2, "episodes": 4, "parse_warnings": 0, "scan_warnings": 1, "duplicates": 1, "duration_ms": 8 }
```

Warning codes:

- `permission_denied` -- a directory or file could not be read.
- `walk_error` -- another filesystem error while traversing.
- `symlink_skipped` -- a symlink was not followed.
- `non_portable_name` -- a name uses characters problematic on Windows (`< > : " \ | ? *`), control characters, a
  trailing space or dot, or a reserved device name (`CON`, `NUL`, `COM1`...).

Parse warnings (`unknown_parse` for an unrecognized name, `ambiguous_parse` for a genuine conflict) are not printed to
stderr individually -- only their count -- but appear in full under each series' `parse_warnings`.

## Supported Formats

Recognized video extensions:

`.mkv` `.mp4` `.avi` `.mov` `.m4v` `.ts` `.wmv` `.webm` `.mpg` `.mpeg` `.m2ts` `.mts` `.vob` `.flv` `.divx`

Any file with one of these extensions is considered, regardless of its name; episode detection is left to the patterns
below.

Recognized episode-numbering patterns:

| Pattern             | Examples                                                        |
| ------------------- | --------------------------------------------------------------- |
| `SxxExx`            | `S01E02`, `S01E02E03`, `S01E01-E03`, `S01E01-03`                |
| `NxNN`              | `1x02`, `1x02x03`, `2x04-06`                                    |
| Season/Episode path | `.../Season 1/Episode 2 - Intro.mkv`                            |
| Specials            | `S00E01`, `.../Specials/Episode 12.mkv`, `Show.Specials.03.mkv` |

Dash ranges expand inclusively (`S01E01-03` --> episodes 1, 2, 3). A descending or implausibly large range is treated as
a single episode, so a stray resolution such as `S01E01-720p` is not read as a 720-episode range. When several patterns
match, the leftmost wins.

## Known Limitations

- The series name comes from the top-level folder under the scanned path. A flat folder holding several shows side by
  side lumps them under one name.
- Episode titles are captured verbatim from the text after the episode token and are not cleaned of release metadata.
- Episode-range support covers `SxxExx` and `NxNN`; date-based and absolute (anime-style) numbering are not parsed.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) and [LICENSE](LICENSE).
