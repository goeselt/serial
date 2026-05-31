package parser

import (
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Status is the outcome of a parse attempt.
type Status string

// Parse status values returned by Parse.
const (
	StatusOK        Status = "ok"
	StatusUnknown   Status = "unknown"
	StatusAmbiguous Status = "ambiguous"
)

// Episode holds the season and episode numbers extracted from a filename.
type Episode struct {
	SeasonNumber  int `json:"season_number"`
	EpisodeNumber int `json:"episode_number"`
}

// Warning describes a parse problem encountered while processing a filename.
type Warning struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Result is returned by Parse and contains the extracted episodes, title, and any warnings.
type Result struct {
	Status   Status    `json:"status"`
	Episodes []Episode `json:"episodes,omitempty"`
	Title    string    `json:"title,omitempty"`
	Warnings []Warning `json:"warnings,omitempty"`
}

type candidate struct {
	pattern  string
	episodes []Episode
	start    int
	end      int
}

// maxRangeSpan caps how many episodes a single dash range may expand to. It
// guards against resolution-like false positives (e.g. "S01E01-720p" where
// "720" is captured as a range end) producing absurd episode lists.
const maxRangeSpan = 64

var (
	// The SxE and x-pattern episode tails accept concatenated multi-episode
	// forms ("E01E02", "1x02x03") via their first alternative and dash ranges
	// ("E01-02", "1x01-02") via the second. Spaces are allowed before the dash
	// but not after it, so an episode title beginning with a number
	// ("S01E01 - 2 Girls") is not mistaken for a range.
	sxePatternRe = regexp.MustCompile(`(?i)\bS(\d{1,2})[ ._-]*E(\d{1,3}(?:[ ._-]*E\d{1,3}|[ ._-]*-[._-]*\d{1,3})*)\b`)
	xPatternRe   = regexp.MustCompile(`(?i)\b(\d{1,2})x(\d{1,3}(?:x\d{1,3}|[ ._-]*-[._-]*\d{1,3})*)\b`)
	// tailTokenRe splits an episode tail into (separator, number) pairs. The
	// optional "e"/"x" is the list-join letter consumed before each number.
	tailTokenRe         = regexp.MustCompile(`(?i)([ ._-]*)[ex]?(\d{1,3})`)
	seasonEpisodePathRe = regexp.MustCompile(`(?i)season[ ._-]*(\d{1,2}).{0,20}?episode[ ._-]*(\d{1,3})`)
	specialsEpisodeRe   = regexp.MustCompile(`(?i)(?:^|[/ ._-])specials(?:[/ ._-]+(?:episode|ep))?[/ ._-]*(\d{1,3})\b`)
)

// Parse extracts season/episode references from a filename or path.
// It applies a left-to-right rule: the first recognized episode pattern wins.
func Parse(input string) Result {
	norm := normalize(input)
	candidates := collectCandidates(norm)
	if len(candidates) == 0 {
		return Result{
			Status: StatusUnknown,
			Warnings: []Warning{{
				Code:    "unknown_parse",
				Message: "no supported episode pattern found",
			}},
		}
	}

	firstStart := minStart(candidates)
	firstCandidates := filterByStart(candidates, firstStart)
	unique := uniqueByEpisodes(firstCandidates)

	if len(unique) > 1 {
		parts := make([]string, 0, len(unique))
		for _, c := range unique {
			parts = append(parts, fmt.Sprintf("%s=%s", c.pattern, episodesKey(c.episodes)))
		}
		sort.Strings(parts)
		return Result{
			Status: StatusAmbiguous,
			Warnings: []Warning{{
				Code:    "ambiguous_parse",
				Message: "conflicting episode patterns at same position: " + strings.Join(parts, "; "),
			}},
		}
	}

	selected := unique[0]
	return Result{
		Status:   StatusOK,
		Episodes: selected.episodes,
		Title:    extractTitle(norm, selected.end),
	}
}

func collectCandidates(norm string) []candidate {
	candidates := parseSXE(norm)
	candidates = append(candidates, parseXPattern(norm)...)
	candidates = append(candidates, parseSeasonEpisodePath(norm)...)
	candidates = append(candidates, parseSpecials(norm)...)
	return candidates
}

func parseSXE(norm string) []candidate {
	matches := sxePatternRe.FindAllStringSubmatchIndex(norm, -1)
	out := make([]candidate, 0, len(matches))
	for _, m := range matches {
		if len(m) < 6 {
			continue
		}
		season, err := strconv.Atoi(norm[m[2]:m[3]])
		if err != nil {
			continue
		}
		episodes := dedupeAndSortEpisodes(episodesFromTail(season, norm[m[4]:m[5]]))
		if len(episodes) > 0 {
			out = append(out, candidate{pattern: "sxe", episodes: episodes, start: m[0], end: m[1]})
		}
	}
	return out
}

// episodesFromTail expands the episode portion of an SxE or x-pattern match into
// concrete episodes. Numbers joined by a dash form an inclusive range
// (E01-03 -> 1,2,3); numbers joined otherwise (E01E02, 1x02x03) are taken
// literally. Ranges that descend or exceed maxRangeSpan drop the trailing
// number, which keeps a stray resolution like "E01-720p" from exploding into
// hundreds of episodes.
func episodesFromTail(season int, tail string) []Episode {
	tokens := tailTokenRe.FindAllStringSubmatch(tail, -1)
	episodes := make([]Episode, 0, len(tokens))
	prev := -1
	for _, t := range tokens {
		sep, numStr := t[1], t[2]
		num, err := strconv.Atoi(numStr)
		if err != nil {
			continue
		}
		if prev >= 0 && strings.Contains(sep, "-") {
			// Dash join: inclusive range from the previous episode. A
			// descending or oversized span is implausible (typically a
			// resolution or year), so drop the number entirely.
			if num > prev && num-prev <= maxRangeSpan {
				for e := prev + 1; e <= num; e++ {
					episodes = append(episodes, Episode{SeasonNumber: season, EpisodeNumber: e})
				}
				prev = num
			}
			continue
		}
		episodes = append(episodes, Episode{SeasonNumber: season, EpisodeNumber: num})
		prev = num
	}
	return episodes
}

func parseXPattern(norm string) []candidate {
	matches := xPatternRe.FindAllStringSubmatchIndex(norm, -1)
	out := make([]candidate, 0, len(matches))
	for _, m := range matches {
		if len(m) < 6 {
			continue
		}
		season, err := strconv.Atoi(norm[m[2]:m[3]])
		if err != nil {
			continue
		}

		episodes := dedupeAndSortEpisodes(episodesFromTail(season, norm[m[4]:m[5]]))
		if len(episodes) > 0 {
			out = append(out, candidate{pattern: "x", episodes: episodes, start: m[0], end: m[1]})
		}
	}
	return out
}

func parseSeasonEpisodePath(norm string) []candidate {
	matches := seasonEpisodePathRe.FindAllStringSubmatchIndex(norm, -1)
	out := make([]candidate, 0, len(matches))
	for _, m := range matches {
		if len(m) < 6 {
			continue
		}
		season, err := strconv.Atoi(norm[m[2]:m[3]])
		if err != nil {
			continue
		}
		ep, err := strconv.Atoi(norm[m[4]:m[5]])
		if err != nil {
			continue
		}
		out = append(out, candidate{
			pattern:  "season_episode",
			episodes: []Episode{{SeasonNumber: season, EpisodeNumber: ep}},
			start:    m[0],
			end:      m[1],
		})
	}
	return out
}

func parseSpecials(norm string) []candidate {
	matches := specialsEpisodeRe.FindAllStringSubmatchIndex(norm, -1)
	out := make([]candidate, 0, len(matches))
	for _, m := range matches {
		if len(m) < 4 {
			continue
		}
		ep, err := strconv.Atoi(norm[m[2]:m[3]])
		if err != nil {
			continue
		}
		out = append(out, candidate{
			pattern:  "specials_episode",
			episodes: []Episode{{SeasonNumber: 0, EpisodeNumber: ep}},
			start:    m[0],
			end:      m[1],
		})
	}
	return out
}

func minStart(candidates []candidate) int {
	m := candidates[0].start
	for i := 1; i < len(candidates); i++ {
		if candidates[i].start < m {
			m = candidates[i].start
		}
	}
	return m
}

func filterByStart(in []candidate, start int) []candidate {
	out := make([]candidate, 0, len(in))
	for _, c := range in {
		if c.start == start {
			out = append(out, c)
		}
	}
	return out
}

func uniqueByEpisodes(in []candidate) []candidate {
	seen := make(map[string]candidate, len(in))
	for _, c := range in {
		key := episodesKey(c.episodes)
		existing, ok := seen[key]
		if !ok {
			seen[key] = c
			continue
		}
		if c.end > existing.end {
			seen[key] = c
		}
	}
	out := make([]candidate, 0, len(seen))
	for _, c := range seen {
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool {
		ki := episodesKey(out[i].episodes)
		kj := episodesKey(out[j].episodes)
		if ki != kj {
			return ki < kj
		}
		if out[i].start != out[j].start {
			return out[i].start < out[j].start
		}
		if out[i].end != out[j].end {
			return out[i].end > out[j].end
		}
		return out[i].pattern < out[j].pattern
	})
	return out
}

func dedupeAndSortEpisodes(in []Episode) []Episode {
	seen := make(map[string]Episode, len(in))
	for _, ep := range in {
		key := fmt.Sprintf("%d-%d", ep.SeasonNumber, ep.EpisodeNumber)
		seen[key] = ep
	}

	out := make([]Episode, 0, len(seen))
	for _, ep := range seen {
		out = append(out, ep)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].SeasonNumber != out[j].SeasonNumber {
			return out[i].SeasonNumber < out[j].SeasonNumber
		}
		return out[i].EpisodeNumber < out[j].EpisodeNumber
	})
	return out
}

func episodesKey(episodes []Episode) string {
	parts := make([]string, 0, len(episodes))
	for _, ep := range episodes {
		parts = append(parts, fmt.Sprintf("S%02dE%03d", ep.SeasonNumber, ep.EpisodeNumber))
	}
	return strings.Join(parts, ",")
}

func extractTitle(norm string, matchEnd int) string {
	if matchEnd < 0 || matchEnd > len(norm) {
		return ""
	}
	rest := norm[matchEnd:]
	rest = strings.TrimSpace(rest)
	rest = strings.TrimLeft(rest, "-._ []()")
	rest = strings.TrimSpace(rest)
	return rest
}

func normalize(in string) string {
	slashed := filepath.ToSlash(in)
	cleaned := filepath.Clean(slashed)
	ext := filepath.Ext(cleaned)
	if ext != "" {
		cleaned = strings.TrimSuffix(cleaned, ext)
	}
	return cleaned
}
