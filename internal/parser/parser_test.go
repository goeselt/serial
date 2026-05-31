package parser

import (
	"reflect"
	"testing"
)

func TestParseSupportedPatterns(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		input         string
		expected      []Episode
		expectedTitle string
	}{
		{
			name:          "sxe single",
			input:         "Show.S01E02.1080p.mkv",
			expected:      []Episode{{SeasonNumber: 1, EpisodeNumber: 2}},
			expectedTitle: "1080p",
		},
		{
			name:  "sxe multi",
			input: "Show.S01E02E03.mkv",
			expected: []Episode{
				{SeasonNumber: 1, EpisodeNumber: 2},
				{SeasonNumber: 1, EpisodeNumber: 3},
			},
			expectedTitle: "",
		},
		{
			name:  "sxe dash range with e",
			input: "Show.S01E01-E03.mkv",
			expected: []Episode{
				{SeasonNumber: 1, EpisodeNumber: 1},
				{SeasonNumber: 1, EpisodeNumber: 2},
				{SeasonNumber: 1, EpisodeNumber: 3},
			},
			expectedTitle: "",
		},
		{
			name:  "sxe dash range without e",
			input: "Show.S02E04-06.Title.mkv",
			expected: []Episode{
				{SeasonNumber: 2, EpisodeNumber: 4},
				{SeasonNumber: 2, EpisodeNumber: 5},
				{SeasonNumber: 2, EpisodeNumber: 6},
			},
			expectedTitle: "Title",
		},
		{
			name:  "sxe dash range adjacent",
			input: "Show.S01E01-02.mkv",
			expected: []Episode{
				{SeasonNumber: 1, EpisodeNumber: 1},
				{SeasonNumber: 1, EpisodeNumber: 2},
			},
			expectedTitle: "",
		},
		{
			name:          "x pattern",
			input:         "Show.1x02.avi",
			expected:      []Episode{{SeasonNumber: 1, EpisodeNumber: 2}},
			expectedTitle: "",
		},
		{
			name:  "x pattern multi",
			input: "Show.1x02x03.avi",
			expected: []Episode{
				{SeasonNumber: 1, EpisodeNumber: 2},
				{SeasonNumber: 1, EpisodeNumber: 3},
			},
			expectedTitle: "",
		},
		{
			name:  "x pattern dash range",
			input: "Show.2x04-06.avi",
			expected: []Episode{
				{SeasonNumber: 2, EpisodeNumber: 4},
				{SeasonNumber: 2, EpisodeNumber: 5},
				{SeasonNumber: 2, EpisodeNumber: 6},
			},
			expectedTitle: "",
		},
		{
			name:          "season episode path",
			input:         "/media/Show/Season 1/Episode 2 - Intro.mkv",
			expected:      []Episode{{SeasonNumber: 1, EpisodeNumber: 2}},
			expectedTitle: "Intro",
		},
		{
			name:          "specials s00",
			input:         "Show.S00E01.mkv",
			expected:      []Episode{{SeasonNumber: 0, EpisodeNumber: 1}},
			expectedTitle: "",
		},
		{
			name:          "specials folder",
			input:         "/media/Show/Specials/Episode 12 - Reunion.mkv",
			expected:      []Episode{{SeasonNumber: 0, EpisodeNumber: 12}},
			expectedTitle: "Reunion",
		},
		{
			name:          "specials number",
			input:         "Show.Specials.03.mkv",
			expected:      []Episode{{SeasonNumber: 0, EpisodeNumber: 3}},
			expectedTitle: "",
		},
		{
			name:          "left to right wins in mixed title",
			input:         "The Wire/The Wire - S00E20 - Season 5 Episode 11 Special - The Last Word.mkv",
			expected:      []Episode{{SeasonNumber: 0, EpisodeNumber: 20}},
			expectedTitle: "Season 5 Episode 11 Special - The Last Word",
		},
		{
			name:          "special in title does not override sxe",
			input:         "A Returner's Magic Should Be Special - S01E01 - Destruction.mkv",
			expected:      []Episode{{SeasonNumber: 1, EpisodeNumber: 1}},
			expectedTitle: "Destruction",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := Parse(tt.input)
			if got.Status != StatusOK {
				t.Fatalf("status mismatch: got %q want %q", got.Status, StatusOK)
			}
			if !reflect.DeepEqual(got.Episodes, tt.expected) {
				t.Fatalf("episodes mismatch: got %#v want %#v", got.Episodes, tt.expected)
			}
			if got.Title != tt.expectedTitle {
				t.Fatalf("title mismatch: got %q want %q", got.Title, tt.expectedTitle)
			}
			if len(got.Warnings) != 0 {
				t.Fatalf("expected no warnings, got %#v", got.Warnings)
			}
		})
	}
}

func TestParseFirstPatternWinsWhenLaterPatternConflicts(t *testing.T) {
	t.Parallel()

	got := Parse("Show.S01E02.1x03.mkv")
	if got.Status != StatusOK {
		t.Fatalf("status mismatch: got %q want %q", got.Status, StatusOK)
	}
	want := []Episode{{SeasonNumber: 1, EpisodeNumber: 2}}
	if !reflect.DeepEqual(got.Episodes, want) {
		t.Fatalf("episodes mismatch: got %#v want %#v", got.Episodes, want)
	}
	if got.Title != "1x03" {
		t.Fatalf("title mismatch: got %q", got.Title)
	}
}

func TestParseDashRangeIgnoresResolutionFalsePositive(t *testing.T) {
	t.Parallel()

	got := Parse("Show.S01E01-720p.mkv")
	if got.Status != StatusOK {
		t.Fatalf("status mismatch: got %q want %q", got.Status, StatusOK)
	}
	want := []Episode{{SeasonNumber: 1, EpisodeNumber: 1}}
	if !reflect.DeepEqual(got.Episodes, want) {
		t.Fatalf("episodes mismatch: got %#v want %#v", got.Episodes, want)
	}
}

func TestParseUnknownName(t *testing.T) {
	t.Parallel()

	got := Parse("Show.The.Complete.Series.mkv")
	if got.Status != StatusUnknown {
		t.Fatalf("status mismatch: got %q want %q", got.Status, StatusUnknown)
	}
	if len(got.Episodes) != 0 {
		t.Fatalf("expected no episodes, got %#v", got.Episodes)
	}
	if len(got.Warnings) != 1 {
		t.Fatalf("warnings length mismatch: got %d", len(got.Warnings))
	}
	if got.Warnings[0].Code != "unknown_parse" {
		t.Fatalf("warning code mismatch: got %q", got.Warnings[0].Code)
	}
}
