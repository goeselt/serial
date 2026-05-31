package scanner

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestScanBuildsLibraryAndDetectsDuplicates(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	seriesDir := filepath.Join(root, "Show")
	if err := os.MkdirAll(filepath.Join(seriesDir, "Specials"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	mustWriteFile(t, filepath.Join(seriesDir, "Show.S01E01.mkv"))
	mustWriteFile(t, filepath.Join(seriesDir, "Show.S01E01E02.mkv"))
	mustWriteFile(t, filepath.Join(seriesDir, "Specials", "Episode 03.mkv"))
	mustWriteFile(t, filepath.Join(seriesDir, "Show.S01E03.srt"))

	res, err := Scan(context.Background(), root)
	if err != nil {
		t.Fatalf("Scan error: %v", err)
	}

	if len(res.Library.Series) != 1 {
		t.Fatalf("series count mismatch: got %d", len(res.Library.Series))
	}

	series := res.Library.Series[0]
	if series.Name != "Show" {
		t.Fatalf("series name mismatch: got %q", series.Name)
	}

	if len(series.Seasons) != 2 {
		t.Fatalf("season count mismatch: got %d", len(series.Seasons))
	}

	if got := len(res.Duplicates); got != 1 {
		t.Fatalf("duplicate groups mismatch: got %d", got)
	}
	dup := res.Duplicates[0]
	if dup.SeasonNumber != 1 || dup.EpisodeNumber != 1 {
		t.Fatalf("duplicate key mismatch: got S%02dE%02d", dup.SeasonNumber, dup.EpisodeNumber)
	}
	if len(dup.Paths) != 2 {
		t.Fatalf("duplicate paths mismatch: got %v", dup.Paths)
	}

	if len(series.ParseNotes) != 0 {
		t.Fatalf("unexpected parse warnings: %v", series.ParseNotes)
	}
}

func TestScanDoesNotReportSplitEpisodeAsDuplicate(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	seriesDir := filepath.Join(root, "SplitShow")
	if err := os.MkdirAll(seriesDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	mustWriteFile(t, filepath.Join(seriesDir, "SplitShow - S01E01 - Pilot (Part 1).mkv"))
	mustWriteFile(t, filepath.Join(seriesDir, "SplitShow - S01E01 - Pilot (Part 2).mkv"))
	mustWriteFile(t, filepath.Join(seriesDir, "SplitShow - S01E02 - Next.mkv"))

	res, err := Scan(context.Background(), root)
	if err != nil {
		t.Fatalf("Scan error: %v", err)
	}

	if got := len(res.Duplicates); got != 0 {
		t.Fatalf("expected no duplicates for split episode, got %d: %#v", got, res.Duplicates)
	}
}

func TestScanSplitEpisodeVariantWithNumberInParentheses(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	seriesDir := filepath.Join(root, "SplitParensShow")
	if err := os.MkdirAll(seriesDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	mustWriteFile(t, filepath.Join(seriesDir, "SplitParensShow - S01E05 - Finale (1).mkv"))
	mustWriteFile(t, filepath.Join(seriesDir, "SplitParensShow - S01E05 - Finale (2).mkv"))

	res, err := Scan(context.Background(), root)
	if err != nil {
		t.Fatalf("Scan error: %v", err)
	}

	if got := len(res.Duplicates); got != 0 {
		t.Fatalf("expected no duplicates for split episode, got %d: %#v", got, res.Duplicates)
	}
}

func TestScanCollectsParseWarnings(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	seriesDir := filepath.Join(root, "WarnShow")
	if err := os.MkdirAll(seriesDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	mustWriteFile(t, filepath.Join(seriesDir, "WarnShow.S01E02.1x03.mkv"))
	mustWriteFile(t, filepath.Join(seriesDir, "WarnShow.Complete.Series.mkv"))

	res, err := Scan(context.Background(), root)
	if err != nil {
		t.Fatalf("Scan error: %v", err)
	}

	if len(res.Library.Series) != 1 {
		t.Fatalf("series count mismatch: got %d", len(res.Library.Series))
	}

	notes := res.Library.Series[0].ParseNotes
	if len(notes) != 1 {
		t.Fatalf("parse warnings mismatch: got %d", len(notes))
	}

	codes := notes[0].Code
	if !strings.Contains(codes, "unknown_parse") {
		t.Fatalf("parse warning codes mismatch: %s", codes)
	}
}

func TestScanSkipsSymlinks(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	seriesDir := filepath.Join(root, "LinkShow")
	if err := os.MkdirAll(seriesDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	target := filepath.Join(seriesDir, "LinkShow.S01E01.mkv")
	mustWriteFile(t, target)

	link := filepath.Join(seriesDir, "LinkShow.S01E02.link.mkv")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlink not supported in environment: %v", err)
	}

	res, err := Scan(context.Background(), root)
	if err != nil {
		t.Fatalf("Scan error: %v", err)
	}

	if len(res.ScanWarnings) == 0 {
		t.Fatalf("expected symlink warning")
	}
	if res.ScanWarnings[0].Code != "symlink_skipped" {
		t.Fatalf("warning code mismatch: got %q", res.ScanWarnings[0].Code)
	}
}

func TestScanUnreadableDirectoryWarning(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == "windows" {
		t.Skip("permission model differs on windows")
	}

	root := t.TempDir()
	seriesDir := filepath.Join(root, "PermShow")
	if err := os.MkdirAll(seriesDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	locked := filepath.Join(seriesDir, "Locked")
	if err := os.MkdirAll(locked, 0o755); err != nil {
		t.Fatalf("mkdir locked: %v", err)
	}
	mustWriteFile(t, filepath.Join(locked, "PermShow.S01E01.mkv"))

	if err := os.Chmod(locked, 0o000); err != nil {
		t.Fatalf("chmod locked: %v", err)
	}
	defer func() {
		_ = os.Chmod(locked, 0o755)
	}()

	res, err := Scan(context.Background(), root)
	if err != nil {
		t.Fatalf("Scan error: %v", err)
	}

	found := false
	for _, w := range res.ScanWarnings {
		if w.Code == "permission_denied" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected permission_denied warning, got %v", res.ScanWarnings)
	}
}

func TestScanReportsProgress(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	seriesDir := filepath.Join(root, "ProgShow")
	if err := os.MkdirAll(seriesDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	mustWriteFile(t, filepath.Join(seriesDir, "ProgShow.S01E01.mkv"))
	mustWriteFile(t, filepath.Join(seriesDir, "ProgShow.S01E02.mkv"))

	events := []Progress{}
	_, err := ScanWithProgress(context.Background(), root, func(p Progress) {
		events = append(events, p)
	})
	if err != nil {
		t.Fatalf("ScanWithProgress error: %v", err)
	}

	if len(events) == 0 {
		t.Fatalf("expected progress events")
	}
	if events[0].Phase != "scan_started" {
		t.Fatalf("first progress phase mismatch: got %q", events[0].Phase)
	}
	if events[len(events)-1].Phase != "scan_completed" {
		t.Fatalf("last progress phase mismatch: got %q", events[len(events)-1].Phase)
	}
}

func mustWriteFile(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir dir: %v", err)
	}
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
}

func TestNonPortableNameReason(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		wantHit bool
	}{
		{name: "plain", input: "Breaking Bad", wantHit: false},
		{name: "dotted release", input: "Breaking.Bad.S01E01.mkv", wantHit: false},
		{name: "colon", input: "Some: Show", wantHit: true},
		{name: "question mark", input: "ep?.mkv", wantHit: true},
		{name: "backslash", input: "a\\b", wantHit: true},
		{name: "pipe", input: "a|b", wantHit: true},
		{name: "trailing space", input: "Show ", wantHit: true},
		{name: "trailing dot", input: "Show.", wantHit: true},
		{name: "control char", input: "a\x01b", wantHit: true},
		{name: "reserved device", input: "CON", wantHit: true},
		{name: "reserved device with ext", input: "nul.mkv", wantHit: true},
		{name: "reserved substring is fine", input: "console.mkv", wantHit: false},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := nonPortableNameReason(tt.input)
			if (got != "") != tt.wantHit {
				t.Errorf("nonPortableNameReason(%q) = %q, want hit=%v", tt.input, got, tt.wantHit)
			}
		})
	}
}

func TestShouldIgnoreFile(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		path       string
		wantIgnore bool
	}{
		{name: "plain episode", path: "Show.S01E02.mkv", wantIgnore: false},
		{name: "non-video extension", path: "Show.S01E02.srt", wantIgnore: true},
		{name: "subtitle extension", path: "Show.S01E02.nfo", wantIgnore: true},
		{name: "sample tag is not special", path: "Show.S01E02-sample.mkv", wantIgnore: false},
		{name: "trailer as title word", path: "MySeries - S01E02 - This is not a trailer.mkv", wantIgnore: false},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := shouldIgnoreFile(tt.path); got != tt.wantIgnore {
				t.Errorf("shouldIgnoreFile(%q) = %v, want %v", tt.path, got, tt.wantIgnore)
			}
		})
	}
}
