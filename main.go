// Command serial scans a local directory for TV series and episodes.
// Progress is written to stderr; results are written to stdout as NDJSON.
//
// Output record types:
//
//	{"type":"series", ...}    -- one per discovered series, sorted by name
//	{"type":"warning", ...}   -- filesystem or parse warnings
//	{"type":"duplicate", ...} -- duplicate episode files
//	{"type":"summary", ...}   -- always the last line
//
// Usage:
//
//	serial [-q] [-n] <path>
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"serial/internal/catalog"
	"serial/internal/scanner"
)

type seriesRecord struct {
	Type          string                 `json:"type"`
	Name          string                 `json:"name"`
	Path          string                 `json:"path,omitempty"`
	Seasons       []catalog.Season       `json:"seasons,omitempty"`
	ParseWarnings []catalog.ParseWarning `json:"parse_warnings,omitempty"`
}

type warningRecord struct {
	Type    string `json:"type"`
	Code    string `json:"code"`
	Message string `json:"message"`
	Path    string `json:"path,omitempty"`
}

type duplicateRecord struct {
	Type          string   `json:"type"`
	SeriesName    string   `json:"series_name"`
	SeasonNumber  int      `json:"season_number"`
	EpisodeNumber int      `json:"episode_number"`
	Paths         []string `json:"paths"`
}

type summaryRecord struct {
	Type          string `json:"type"`
	Series        int    `json:"series"`
	Episodes      int    `json:"episodes"`
	ParseWarnings int    `json:"parse_warnings"`
	ScanWarnings  int    `json:"scan_warnings"`
	Duplicates    int    `json:"duplicates"`
	DurationMS    int64  `json:"duration_ms"`
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("serial", flag.ContinueOnError)
	flags.SetOutput(stderr)
	quiet := flags.Bool("q", false, "suppress progress output on stderr")
	noJSON := flags.Bool("n", false, "suppress JSON output on stdout (stderr only)")
	flags.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "usage: serial [-q] [-n] <path>\n")
		flags.PrintDefaults()
	}

	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 1 {
		flags.Usage()
		return 2
	}
	root := flags.Arg(0)

	start := time.Now()

	progress := func(p scanner.Progress) {
		switch p.Phase {
		case "scan_started", "scan_completed":
		default:
			_, _ = fmt.Fprintf(stderr, "serial: %d files visited -- %d series, %d warnings\n",
				p.VisitedFiles, p.SeriesCount, p.ScanWarnings+p.ParseWarnings)
		}
	}
	if *quiet {
		progress = nil
	}

	if !*quiet {
		_, _ = fmt.Fprintf(stderr, "serial: scanning %s\n", root)
	}

	if _, statErr := os.Stat(root); statErr != nil {
		_, _ = fmt.Fprintf(stderr, "serial: %v\n", statErr)
		return 1
	}

	result, err := scanner.ScanWithProgress(context.Background(), root, progress)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "serial: error: %v\n", err)
		return 1
	}

	out := stdout
	if *noJSON {
		out = io.Discard
	}
	enc := json.NewEncoder(out)

	totalEpisodes := 0
	totalParseWarnings := 0
	for _, s := range result.Library.Series {
		for _, season := range s.Seasons {
			totalEpisodes += len(season.Episodes)
		}
		totalParseWarnings += len(s.ParseNotes)

		rec := seriesRecord{
			Type:    "series",
			Name:    s.Name,
			Path:    s.Path,
			Seasons: s.Seasons,
		}
		if len(s.ParseNotes) > 0 {
			rec.ParseWarnings = s.ParseNotes
		}
		if encErr := enc.Encode(rec); encErr != nil {
			_, _ = fmt.Fprintf(stderr, "serial: encode error: %v\n", encErr)
			return 1
		}
	}

	for _, w := range result.ScanWarnings {
		if encErr := enc.Encode(warningRecord{
			Type:    "warning",
			Code:    w.Code,
			Message: w.Message,
			Path:    w.Path,
		}); encErr != nil {
			_, _ = fmt.Fprintf(stderr, "serial: encode error: %v\n", encErr)
			return 1
		}
		if !*quiet {
			_, _ = fmt.Fprintf(stderr, "serial: warning %s: %s: %s\n", w.Code, w.Path, w.Message)
		}
	}

	for _, d := range result.Duplicates {
		if encErr := enc.Encode(duplicateRecord{
			Type:          "duplicate",
			SeriesName:    d.SeriesName,
			SeasonNumber:  d.SeasonNumber,
			EpisodeNumber: d.EpisodeNumber,
			Paths:         d.Paths,
		}); encErr != nil {
			_, _ = fmt.Fprintf(stderr, "serial: encode error: %v\n", encErr)
			return 1
		}
		if !*quiet {
			_, _ = fmt.Fprintf(stderr, "serial: duplicate: %s S%02dE%02d -- %s\n",
				d.SeriesName, d.SeasonNumber, d.EpisodeNumber, strings.Join(d.Paths, ", "))
		}
	}

	summary := summaryRecord{
		Type:          "summary",
		Series:        len(result.Library.Series),
		Episodes:      totalEpisodes,
		ParseWarnings: totalParseWarnings,
		ScanWarnings:  len(result.ScanWarnings),
		Duplicates:    len(result.Duplicates),
		DurationMS:    time.Since(start).Milliseconds(),
	}
	if encErr := enc.Encode(summary); encErr != nil {
		_, _ = fmt.Fprintf(stderr, "serial: encode error: %v\n", encErr)
		return 1
	}

	if !*quiet {
		_, _ = fmt.Fprintf(stderr, "serial: done -- %d series, %d episodes, %d duplicates, %d parse warnings, %d scan warnings (%s)\n",
			summary.Series, summary.Episodes, summary.Duplicates, summary.ParseWarnings, summary.ScanWarnings,
			time.Since(start).Round(time.Millisecond))
	}

	return 0
}
