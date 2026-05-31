package catalog

import "sort"

// LocalLibrary is the local scan result root model.
type LocalLibrary struct {
	Series []Series `json:"series"`
}

// Series groups seasons for one show.
type Series struct {
	Name       string   `json:"name"`
	Path       string   `json:"path,omitempty"`
	Seasons    []Season `json:"seasons,omitempty"`
	ParseNotes []ParseWarning
}

// Season groups episodes by season number.
type Season struct {
	Number   int       `json:"number"`
	Episodes []Episode `json:"episodes,omitempty"`
}

// Episode represents one parsed episode.
type Episode struct {
	Number int    `json:"number"`
	Title  string `json:"title,omitempty"`
	Path   string `json:"path,omitempty"`
}

// ParseWarning is emitted by filename parsing.
type ParseWarning struct {
	Path    string `json:"path,omitempty"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

// ScanWarning is emitted by filesystem walking.
type ScanWarning struct {
	Path    string `json:"path,omitempty"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

// DuplicateGroup groups duplicate episode files.
type DuplicateGroup struct {
	SeriesName    string   `json:"series_name,omitempty"`
	SeasonNumber  int      `json:"season_number"`
	EpisodeNumber int      `json:"episode_number"`
	Paths         []string `json:"paths,omitempty"`
}

// SortLibrary normalizes ordering for deterministic output.
func SortLibrary(lib *LocalLibrary) {
	if lib == nil {
		return
	}
	for i := range lib.Series {
		sortSeries(&lib.Series[i])
	}
	sort.Slice(lib.Series, func(i, j int) bool {
		a := lib.Series[i]
		b := lib.Series[j]
		if a.Name != b.Name {
			return a.Name < b.Name
		}
		return a.Path < b.Path
	})
}

func sortSeries(s *Series) {
	sort.Slice(s.ParseNotes, func(i, j int) bool {
		if s.ParseNotes[i].Code != s.ParseNotes[j].Code {
			return s.ParseNotes[i].Code < s.ParseNotes[j].Code
		}
		if s.ParseNotes[i].Path != s.ParseNotes[j].Path {
			return s.ParseNotes[i].Path < s.ParseNotes[j].Path
		}
		return s.ParseNotes[i].Message < s.ParseNotes[j].Message
	})

	for i := range s.Seasons {
		sort.Slice(s.Seasons[i].Episodes, func(a, b int) bool {
			ea := s.Seasons[i].Episodes[a]
			eb := s.Seasons[i].Episodes[b]
			if ea.Number != eb.Number {
				return ea.Number < eb.Number
			}
			return ea.Path < eb.Path
		})
	}

	sort.Slice(s.Seasons, func(i, j int) bool {
		return s.Seasons[i].Number < s.Seasons[j].Number
	})
}
