package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeFile creates a file at path with content "x", creating parent dirs.
func writeFile(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
}

// parseNDJSON decodes all lines from r into a slice of raw JSON objects.
func parseNDJSON(t *testing.T, r io.Reader) []map[string]any {
	t.Helper()
	var recs []map[string]any
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		var rec map[string]any
		if err := json.Unmarshal(sc.Bytes(), &rec); err != nil {
			t.Fatalf("invalid JSON line %q: %v", sc.Text(), err)
		}
		recs = append(recs, rec)
	}
	return recs
}

func TestRunMissingArg(t *testing.T) {
	t.Parallel()
	if got := run(nil, io.Discard, io.Discard); got != 2 {
		t.Fatalf("exit code: got %d, want 2", got)
	}
}

func TestRunNonExistentPath(t *testing.T) {
	t.Parallel()
	if got := run([]string{"/nonexistent/serial-test-12345"}, io.Discard, io.Discard); got != 1 {
		t.Fatalf("exit code: got %d, want 1", got)
	}
}

func TestRunOutputsNDJSON(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "Test Show", "Season 1", "Test.Show.S01E01.Pilot.mkv"))
	writeFile(t, filepath.Join(root, "Test Show", "Season 1", "Test.Show.S01E02-03.mkv"))

	var stdout, stderr bytes.Buffer
	if got := run([]string{root}, &stdout, &stderr); got != 0 {
		t.Fatalf("exit code: got %d, want 0; stderr: %s", got, stderr.String())
	}

	recs := parseNDJSON(t, &stdout)
	if len(recs) == 0 {
		t.Fatal("no records in output")
	}

	// Last record must be summary.
	last := recs[len(recs)-1]
	if got := last["type"]; got != "summary" {
		t.Fatalf("last record type: got %q, want %q", got, "summary")
	}
	if got := last["series"].(float64); got != 1 {
		t.Fatalf("summary.series: got %v, want 1", got)
	}
	if got := last["episodes"].(float64); got != 3 {
		t.Fatalf("summary.episodes: got %v, want 3", got)
	}

	// First record must be the series.
	first := recs[0]
	if got := first["type"]; got != "series" {
		t.Fatalf("first record type: got %q, want %q", got, "series")
	}
	if got := first["name"]; got != "Test Show" {
		t.Fatalf("series name: got %q, want %q", got, "Test Show")
	}
}

func TestRunQuietSuppressesStderr(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	var stdout, stderr bytes.Buffer
	if got := run([]string{"-q", root}, &stdout, &stderr); got != 0 {
		t.Fatalf("exit code: got %d, want 0", got)
	}
	if stderr.Len() != 0 {
		t.Fatalf("-q: stderr not empty: %q", stderr.String())
	}
	if stdout.Len() == 0 {
		t.Fatal("-q: stdout must still contain NDJSON")
	}
}

func TestRunNoJSONSuppressesStdout(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	var stdout, stderr bytes.Buffer
	if got := run([]string{"-n", root}, &stdout, &stderr); got != 0 {
		t.Fatalf("exit code: got %d, want 0", got)
	}
	if stdout.Len() != 0 {
		t.Fatalf("-n: stdout not empty: %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "serial:") {
		t.Fatalf("-n: stderr missing progress output: %q", stderr.String())
	}
}

func TestRunSummaryIsLastLine(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "Alpha", "Season 1", "Alpha.S01E01.mkv"))
	writeFile(t, filepath.Join(root, "Beta", "Season 2", "Beta.S02E05.mkv"))

	var stdout bytes.Buffer
	if got := run([]string{"-q", root}, &stdout, io.Discard); got != 0 {
		t.Fatalf("exit code: got %d, want 0", got)
	}

	recs := parseNDJSON(t, &stdout)
	if got := recs[len(recs)-1]["type"]; got != "summary" {
		t.Fatalf("last record type: got %q, want %q", got, "summary")
	}
}

func TestRunDuplicateDetected(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	dir := filepath.Join(root, "Show", "Season 1")
	writeFile(t, filepath.Join(dir, "Show.S01E01.mkv"))
	writeFile(t, filepath.Join(dir, "Show.S01E01.avi"))

	var stdout bytes.Buffer
	if got := run([]string{"-q", root}, &stdout, io.Discard); got != 0 {
		t.Fatalf("exit code: got %d, want 0", got)
	}

	recs := parseNDJSON(t, &stdout)
	var dupCount int
	for _, r := range recs {
		if r["type"] == "duplicate" {
			dupCount++
		}
	}
	if dupCount != 1 {
		t.Fatalf("duplicate records: got %d, want 1", dupCount)
	}
}
