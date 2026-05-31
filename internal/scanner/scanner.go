package scanner

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"serial/internal/catalog"
	"serial/internal/parser"
)

var includeExtensions = map[string]struct{}{
	".mkv":  {},
	".mp4":  {},
	".avi":  {},
	".mov":  {},
	".m4v":  {},
	".ts":   {},
	".wmv":  {},
	".webm": {},
	".mpg":  {},
	".mpeg": {},
	".m2ts": {},
	".mts":  {},
	".vob":  {},
	".flv":  {},
	".divx": {},
}

// windowsReservedNames lists base names (case-insensitive, extension ignored)
// that are reserved device names on Windows and cannot be used as file names.
var windowsReservedNames = map[string]struct{}{
	"con": {}, "prn": {}, "aux": {}, "nul": {},
	"com1": {}, "com2": {}, "com3": {}, "com4": {}, "com5": {},
	"com6": {}, "com7": {}, "com8": {}, "com9": {},
	"lpt1": {}, "lpt2": {}, "lpt3": {}, "lpt4": {}, "lpt5": {},
	"lpt6": {}, "lpt7": {}, "lpt8": {}, "lpt9": {},
}

var (
	partSuffixParenRe = regexp.MustCompile(`(?i)^(.*?)[\s._-]*\((?:part[\s._-]*)?(\d{1,3})\)\s*$`)
	partSuffixWordRe  = regexp.MustCompile(`(?i)^(.*?)[\s._-]*part[\s._-]*(\d{1,3})\s*$`)
	nonWordRunesRe    = regexp.MustCompile(`[^[:alnum:]]+`)
)

// Result holds the complete output of a directory scan.
type Result struct {
	Library      catalog.LocalLibrary
	Duplicates   []catalog.DuplicateGroup
	ScanWarnings []catalog.ScanWarning
}

// Progress carries incremental scan metrics delivered to the onProgress callback.
type Progress struct {
	Phase          string
	VisitedFiles   int
	MediaFiles     int
	ParsedEpisodes int
	ParseWarnings  int
	SeriesCount    int
	ScanWarnings   int
}

// Scan walks root and returns the full scan result.
func Scan(ctx context.Context, root string) (Result, error) {
	return ScanWithProgress(ctx, root, nil)
}

// ScanWithProgress walks root and calls onProgress (when non-nil) after each file batch.
func ScanWithProgress(ctx context.Context, root string, onProgress func(Progress)) (Result, error) {
	if err := ctx.Err(); err != nil {
		return Result{}, fmt.Errorf("scanner context: %w", err)
	}

	seriesByName := map[string]*catalog.Series{}
	scanWarnings := []catalog.ScanWarning{}
	visitedFiles := 0
	mediaFiles := 0
	parsedEpisodes := 0
	parseWarnings := 0
	lastProgress := time.Now()

	reportProgress := func(phase string, force bool) {
		if onProgress == nil {
			return
		}
		if !force && time.Since(lastProgress) < 2*time.Second {
			return
		}
		lastProgress = time.Now()
		onProgress(Progress{
			Phase:          phase,
			VisitedFiles:   visitedFiles,
			MediaFiles:     mediaFiles,
			ParsedEpisodes: parsedEpisodes,
			ParseWarnings:  parseWarnings,
			SeriesCount:    len(seriesByName),
			ScanWarnings:   len(scanWarnings),
		})
	}

	reportProgress("scan_started", true)

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if err := ctx.Err(); err != nil {
			return err
		}

		relPath, err := filepath.Rel(root, path)
		if err != nil {
			relPath = path
		}
		relPath = normalizeRelPath(relPath)

		if walkErr != nil {
			scanWarnings = append(scanWarnings, catalog.ScanWarning{
				Path:    relPath,
				Code:    classifyWalkErr(walkErr),
				Message: walkErr.Error(),
			})
			if d != nil && d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		if d == nil {
			return nil
		}

		if d.Type()&fs.ModeSymlink != 0 {
			scanWarnings = append(scanWarnings, catalog.ScanWarning{
				Path:    relPath,
				Code:    "symlink_skipped",
				Message: "symlink entry skipped",
			})
			return nil
		}

		// Check each entry's own name once so a non-portable folder is
		// reported when the directory is visited, not for every child.
		if relPath != "." {
			if msg := nonPortableNameReason(d.Name()); msg != "" {
				scanWarnings = append(scanWarnings, catalog.ScanWarning{
					Path:    relPath,
					Code:    "non_portable_name",
					Message: msg,
				})
			}
		}

		if d.IsDir() {
			return nil
		}

		visitedFiles++
		reportProgress("scanning", false)

		if shouldIgnoreFile(relPath) {
			return nil
		}
		mediaFiles++

		seriesName := seriesNameFromRelativePath(relPath, root)
		series := ensureSeries(seriesByName, seriesName, root)

		parse := parser.Parse(relPath)
		switch parse.Status {
		case parser.StatusUnknown, parser.StatusAmbiguous:
			parseWarnings += len(parse.Warnings)
			for _, w := range parse.Warnings {
				series.ParseNotes = append(series.ParseNotes, catalog.ParseWarning{
					Path:    relPath,
					Code:    w.Code,
					Message: w.Message,
				})
			}
			return nil
		case parser.StatusOK:
			parsedEpisodes += len(parse.Episodes)
			for _, ep := range parse.Episodes {
				addEpisode(series, ep.SeasonNumber, ep.EpisodeNumber, parse.Title, relPath)
			}
			return nil
		default:
			series.ParseNotes = append(series.ParseNotes, catalog.ParseWarning{
				Path:    relPath,
				Code:    "unknown_parse_status",
				Message: fmt.Sprintf("unsupported parse status %q", parse.Status),
			})
			return nil
		}
	})
	if err != nil {
		return Result{}, fmt.Errorf("scanner walk %s: %w", root, err)
	}

	library := catalog.LocalLibrary{Series: make([]catalog.Series, 0, len(seriesByName))}
	for _, s := range seriesByName {
		library.Series = append(library.Series, *s)
	}

	catalog.SortLibrary(&library)
	sortScanWarnings(scanWarnings)

	duplicates := detectDuplicates(library)
	sortDuplicateGroups(duplicates)

	reportProgress("scan_completed", true)

	return Result{
		Library:      library,
		Duplicates:   duplicates,
		ScanWarnings: scanWarnings,
	}, nil
}

func ensureSeries(seriesByName map[string]*catalog.Series, seriesName, root string) *catalog.Series {
	if s, ok := seriesByName[seriesName]; ok {
		return s
	}
	s := &catalog.Series{
		Name: seriesName,
		Path: filepath.ToSlash(filepath.Join(root, seriesName)),
	}
	seriesByName[seriesName] = s
	return s
}

func addEpisode(series *catalog.Series, seasonNumber, episodeNumber int, title, relPath string) {
	ep := catalog.Episode{

		Number: episodeNumber,
		Title:  title,
		Path:   relPath,
	}
	for i := range series.Seasons {
		if series.Seasons[i].Number == seasonNumber {
			series.Seasons[i].Episodes = append(series.Seasons[i].Episodes, ep)
			return
		}
	}
	series.Seasons = append(series.Seasons, catalog.Season{
		Number:   seasonNumber,
		Episodes: []catalog.Episode{ep},
	})
}

// nonPortableNameReason reports why a single path component is problematic on
// Linux or Windows, or "" when the name is portable. It flags characters
// Windows forbids (Linux only forbids "/" and NUL), trailing spaces or dots,
// and reserved Windows device names.
func nonPortableNameReason(name string) string {
	for _, r := range name {
		if r < 0x20 {
			return "name contains a control character"
		}
		switch r {
		case '<', '>', ':', '"', '\\', '|', '?', '*':
			return fmt.Sprintf("name contains reserved character %q", string(r))
		}
	}

	if strings.HasSuffix(name, " ") || strings.HasSuffix(name, ".") {
		return "name ends with a space or dot"
	}

	stem := strings.ToLower(name)
	if dot := strings.IndexByte(stem, '.'); dot >= 0 {
		stem = stem[:dot]
	}
	if _, ok := windowsReservedNames[stem]; ok {
		return "name is a reserved Windows device name"
	}

	return ""
}

func shouldIgnoreFile(relPath string) bool {
	ext := strings.ToLower(filepath.Ext(relPath))
	if _, ok := includeExtensions[ext]; !ok {
		return true
	}

	baseLower := strings.ToLower(filepath.Base(relPath))
	if strings.Contains(baseLower, "sample") || strings.Contains(baseLower, "trailer") {
		return true
	}

	return false
}

func seriesNameFromRelativePath(relPath, root string) string {
	if relPath == "." || relPath == "" {
		return filepath.Base(root)
	}
	parts := strings.Split(relPath, "/")
	if len(parts) == 1 {
		return filepath.Base(root)
	}
	if parts[0] == "" || parts[0] == "." {
		return filepath.Base(root)
	}
	return parts[0]
}

func normalizeRelPath(rel string) string {
	return filepath.ToSlash(filepath.Clean(rel))
}

func detectDuplicates(library catalog.LocalLibrary) []catalog.DuplicateGroup {
	out := []catalog.DuplicateGroup{}
	for _, series := range library.Series {
		for _, season := range series.Seasons {
			byEpisode := map[string][]catalog.Episode{}
			for _, ep := range season.Episodes {
				key := fmt.Sprintf("%d", ep.Number)
				byEpisode[key] = append(byEpisode[key], ep)
			}

			for _, ep := range season.Episodes {
				key := fmt.Sprintf("%d", ep.Number)
				group := byEpisode[key]
				paths := make([]string, 0, len(group))
				for _, candidate := range group {
					paths = append(paths, candidate.Path)
				}
				paths = dedupeStrings(paths)
				if len(paths) <= 1 {
					continue
				}
				if isSplitEpisodeGroup(group) {
					delete(byEpisode, key)
					continue
				}

				out = append(out, catalog.DuplicateGroup{
					SeriesName:    series.Name,
					SeasonNumber:  season.Number,
					EpisodeNumber: ep.Number,
					Paths:         paths,
				})
				delete(byEpisode, key)
			}
		}
	}
	return out
}

func isSplitEpisodeGroup(group []catalog.Episode) bool {
	type splitCandidate struct {
		base string
		part int
	}

	byPath := map[string]splitCandidate{}
	for _, ep := range group {
		if _, exists := byPath[ep.Path]; exists {
			continue
		}

		rawTitle := strings.TrimSpace(ep.Title)
		if rawTitle == "" {
			base := filepath.Base(ep.Path)
			rawTitle = strings.TrimSuffix(base, filepath.Ext(base))
		}

		base, part, ok := extractSplitPart(rawTitle)
		if !ok {
			return false
		}
		byPath[ep.Path] = splitCandidate{base: base, part: part}
	}

	if len(byPath) < 2 {
		return false
	}

	baseValue := ""
	parts := map[int]struct{}{}
	for _, c := range byPath {
		if baseValue == "" {
			baseValue = c.base
		} else if c.base != baseValue {
			return false
		}
		if _, exists := parts[c.part]; exists {
			return false
		}
		parts[c.part] = struct{}{}
	}
	return true
}

func extractSplitPart(title string) (string, int, bool) {
	title = strings.TrimSpace(title)
	if title == "" {
		return "", 0, false
	}

	if m := partSuffixParenRe.FindStringSubmatch(title); len(m) == 3 {
		part, err := strconv.Atoi(m[2])
		if err == nil {
			base := normalizeSplitBase(m[1])
			if base != "" {
				return base, part, true
			}
		}
	}
	if m := partSuffixWordRe.FindStringSubmatch(title); len(m) == 3 {
		part, err := strconv.Atoi(m[2])
		if err == nil {
			base := normalizeSplitBase(m[1])
			if base != "" {
				return base, part, true
			}
		}
	}

	return "", 0, false
}

func normalizeSplitBase(in string) string {
	normalized := strings.ToLower(strings.TrimSpace(in))
	normalized = nonWordRunesRe.ReplaceAllString(normalized, " ")
	return strings.Join(strings.Fields(normalized), " ")
}

func dedupeStrings(in []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(in))
	for _, v := range in {
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}

func sortScanWarnings(warnings []catalog.ScanWarning) {
	sort.Slice(warnings, func(i, j int) bool {
		if warnings[i].Code != warnings[j].Code {
			return warnings[i].Code < warnings[j].Code
		}
		if warnings[i].Path != warnings[j].Path {
			return warnings[i].Path < warnings[j].Path
		}
		return warnings[i].Message < warnings[j].Message
	})
}

func sortDuplicateGroups(in []catalog.DuplicateGroup) {
	sort.Slice(in, func(i, j int) bool {
		a := in[i]
		b := in[j]
		if a.SeriesName != b.SeriesName {
			return a.SeriesName < b.SeriesName
		}
		if a.SeasonNumber != b.SeasonNumber {
			return a.SeasonNumber < b.SeasonNumber
		}
		return a.EpisodeNumber < b.EpisodeNumber
	})
	for i := range in {
		sort.Strings(in[i].Paths)
	}
}

func classifyWalkErr(err error) string {
	if errors.Is(err, fs.ErrPermission) {
		return "permission_denied"
	}
	return "walk_error"
}
