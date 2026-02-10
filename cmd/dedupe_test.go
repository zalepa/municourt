package cmd

import (
	"testing"

	"github.com/zalepa/municourt/parser"
)

func TestStripMunicipalSuffix(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"GUTTENBERG TOWN", "GUTTENBERG"},
		{"GUTTENBERG", "GUTTENBERG"},
		{"EGG HARBOR CITY", "EGG HARBOR"},
		{"EGG HARBOR TWP", "EGG HARBOR"},
		{"WEST ORANGE TOWNSHIP", "WEST ORANGE"},
		{"WOODBRIDGE BORO", "WOODBRIDGE"},
		{"WOODBRIDGE BOROUGH", "WOODBRIDGE"},
		{"SPRING LAKE VILLAGE", "SPRING LAKE"},
		{"ATLANTIC CITY", "ATLANTIC"},
		// Case insensitive.
		{"guttenberg town", "GUTTENBERG"},
		// No suffix.
		{"ABSECON", "ABSECON"},
		// "TOWN" inside a name shouldn't be stripped.
		{"MORRISTOWN", "MORRISTOWN"},
	}
	for _, tt := range tests {
		got := stripMunicipalSuffix(tt.input)
		if got != tt.want {
			t.Errorf("stripMunicipalSuffix(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func stat(county, muni string) parser.MunicipalityStats {
	return parser.MunicipalityStats{County: county, Municipality: muni}
}

func TestFindDuplicates_NoOverlap(t *testing.T) {
	// GUTTENBERG TOWN appears in 2005-2008, GUTTENBERG in 2010+.
	// They should be flagged as duplicates.
	parsed := []parseResult{
		{inputPath: "muni-2005-07.pdf", date: "2005-07", results: []parser.MunicipalityStats{
			stat("HUDSON", "GUTTENBERG TOWN"),
		}},
		{inputPath: "muni-2006-07.pdf", date: "2006-07", results: []parser.MunicipalityStats{
			stat("HUDSON", "GUTTENBERG TOWN"),
		}},
		{inputPath: "muni-2010-07.pdf", date: "2010-07", results: []parser.MunicipalityStats{
			stat("HUDSON", "GUTTENBERG"),
		}},
		{inputPath: "muni-2011-07.pdf", date: "2011-07", results: []parser.MunicipalityStats{
			stat("HUDSON", "GUTTENBERG"),
		}},
	}

	candidates := findDuplicates(parsed)
	if len(candidates) != 1 {
		t.Fatalf("got %d candidates, want 1", len(candidates))
	}
	c := candidates[0]
	if c.county != "HUDSON" {
		t.Errorf("county = %q, want HUDSON", c.county)
	}
	// Keeper should be GUTTENBERG (more recent data).
	if c.nameA != "GUTTENBERG" {
		t.Errorf("nameA = %q, want GUTTENBERG", c.nameA)
	}
	if c.nameB != "GUTTENBERG TOWN" {
		t.Errorf("nameB = %q, want GUTTENBERG TOWN", c.nameB)
	}
}

func TestFindDuplicates_WithOverlap(t *testing.T) {
	// EGG HARBOR CITY and EGG HARBOR TWP overlap — they are distinct entities.
	parsed := []parseResult{
		{inputPath: "muni-2005-07.pdf", date: "2005-07", results: []parser.MunicipalityStats{
			stat("ATLANTIC", "EGG HARBOR CITY"),
			stat("ATLANTIC", "EGG HARBOR TWP"),
		}},
		{inputPath: "muni-2010-07.pdf", date: "2010-07", results: []parser.MunicipalityStats{
			stat("ATLANTIC", "EGG HARBOR CITY"),
			stat("ATLANTIC", "EGG HARBOR TWP"),
		}},
	}

	candidates := findDuplicates(parsed)
	if len(candidates) != 0 {
		t.Fatalf("got %d candidates, want 0 (overlapping entities are distinct)", len(candidates))
	}
}

func TestFindDuplicates_DifferentCounties(t *testing.T) {
	// Same stripped name but different counties — should not be flagged.
	parsed := []parseResult{
		{inputPath: "muni-2005-07.pdf", date: "2005-07", results: []parser.MunicipalityStats{
			stat("HUDSON", "GUTTENBERG TOWN"),
		}},
		{inputPath: "muni-2010-07.pdf", date: "2010-07", results: []parser.MunicipalityStats{
			stat("BERGEN", "GUTTENBERG"),
		}},
	}

	candidates := findDuplicates(parsed)
	if len(candidates) != 0 {
		t.Fatalf("got %d candidates, want 0 (different counties)", len(candidates))
	}
}

func TestFindDuplicates_SkipsFailedAndDateless(t *testing.T) {
	parsed := []parseResult{
		{inputPath: "bad.pdf", date: "", failed: true, results: []parser.MunicipalityStats{
			stat("HUDSON", "GUTTENBERG TOWN"),
		}},
		{inputPath: "nodate.pdf", date: "", results: []parser.MunicipalityStats{
			stat("HUDSON", "GUTTENBERG"),
		}},
	}

	candidates := findDuplicates(parsed)
	if len(candidates) != 0 {
		t.Fatalf("got %d candidates, want 0 (no usable dates)", len(candidates))
	}
}

func TestFindDuplicates_KeeperIsMoreRecent(t *testing.T) {
	// The name with more recent data should be the keeper (nameA).
	parsed := []parseResult{
		{inputPath: "muni-2020-07.pdf", date: "2020-07", results: []parser.MunicipalityStats{
			stat("PASSAIC", "CLIFTON CITY"),
		}},
		{inputPath: "muni-2005-07.pdf", date: "2005-07", results: []parser.MunicipalityStats{
			stat("PASSAIC", "CLIFTON"),
		}},
	}

	candidates := findDuplicates(parsed)
	if len(candidates) != 1 {
		t.Fatalf("got %d candidates, want 1", len(candidates))
	}
	if candidates[0].nameA != "CLIFTON CITY" {
		t.Errorf("nameA = %q, want CLIFTON CITY (more recent)", candidates[0].nameA)
	}
}
