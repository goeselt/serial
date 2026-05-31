package catalog

import (
	"testing"
)

func TestSortLibraryDeterministic(t *testing.T) {
	t.Parallel()

	lib := &LocalLibrary{
		Series: []Series{
			{
				Name: "Alpha",
				Path: "/media/b",
				Seasons: []Season{
					{Number: 2},
					{
						Number: 1,
						Episodes: []Episode{
							{Number: 2, Path: "z.mkv"},
							{Number: 1, Path: "a.mkv"},
						},
					},
				},
			},
			{Name: "Alpha", Path: "/media/a"},
			{Name: "Beta", Path: "/media/c"},
		},
	}

	SortLibrary(lib)

	if got, want := lib.Series[0].Path, "/media/a"; got != want {
		t.Fatalf("series order mismatch: got %q want %q", got, want)
	}
	if got, want := lib.Series[1].Seasons[0].Number, 1; got != want {
		t.Fatalf("season order mismatch: got %d want %d", got, want)
	}
	if got, want := lib.Series[1].Seasons[0].Episodes[0].Number, 1; got != want {
		t.Fatalf("episode order mismatch: got %d want %d", got, want)
	}
}
