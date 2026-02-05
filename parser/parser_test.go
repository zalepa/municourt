package parser

import (
	"reflect"
	"testing"
)

func TestGroupIntoLines(t *testing.T) {
	items := []string{"", "A", "B", "", "C", "", "", "D", "E", "F", ""}
	got := groupIntoLines(items)
	want := [][]string{{"A", "B"}, {"C"}, {"D", "E", "F"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestMatchSectionName(t *testing.T) {
	tests := []struct {
		line []string
		want string
	}{
		{[]string{"Filings"}, "Filings"},
		{[]string{"Clearance", "Percent"}, "Clearance Percent"},
		{[]string{"Backlog/100", "Mthly", "Filings"}, "Backlog/100 Mthly Filings"},
		{[]string{"Active", "Pending"}, "Active Pending"},
		{[]string{"NotASection"}, ""},
		{[]string{"100%"}, ""},
	}
	for _, tt := range tests {
		got := matchSectionName(tt.line)
		if got != tt.want {
			t.Errorf("matchSectionName(%v) = %q, want %q", tt.line, got, tt.want)
		}
	}
}

func TestMergeCommaSplitNumbers(t *testing.T) {
	tests := []struct {
		name     string
		line     []string
		expected int
		want     []string
	}{
		{
			name:     "no merge needed",
			line:     []string{"label", "434", "385", "77", "896", "33", "2,339", "56", "2,428", "3,324"},
			expected: 10,
			want:     []string{"label", "434", "385", "77", "896", "33", "2,339", "56", "2,428", "3,324"},
		},
		{
			name:     "merge 1,000 (leading zero in right)",
			line:     []string{"label", "434", "385", "77", "896", "33", "1", "000", "56", "2,428", "3,324"},
			expected: 10,
			want:     []string{"label", "434", "385", "77", "896", "33", "1,000", "56", "2,428", "3,324"},
		},
		{
			name:     "merge 1,040 (leading zero in right)",
			line:     []string{"label", "434", "385", "77", "896", "33", "1", "040", "56", "2,428", "3,324"},
			expected: 10,
			want:     []string{"label", "434", "385", "77", "896", "33", "1,040", "56", "2,428", "3,324"},
		},
		{
			name:     "merge two splits: 1,000 and 2,090",
			line:     []string{"label", "1", "000", "385", "77", "896", "33", "2", "090", "56", "2,428", "3,324"},
			expected: 10,
			want:     []string{"label", "1,000", "385", "77", "896", "33", "2,090", "56", "2,428", "3,324"},
		},
		{
			name:     "merge prefers leading zero over digit-length",
			line:     []string{"label", "12", "345", "385", "77", "896", "33", "1", "090", "56", "2,428"},
			expected: 10,
			want:     []string{"label", "12", "345", "385", "77", "896", "33", "1,090", "56", "2,428"},
		},
		{
			name:     "merge by digit-length when no leading zero available",
			line:     []string{"label", "5", "800", "385", "77", "896", "33", "100", "56", "2,428", "3,324"},
			expected: 10,
			want:     []string{"label", "5,800", "385", "77", "896", "33", "100", "56", "2,428", "3,324"},
		},
		{
			name:     "no merge when already correct length",
			line:     []string{"label", "1", "000", "385", "77", "896", "33", "100", "56"},
			expected: 9,
			want:     []string{"label", "1", "000", "385", "77", "896", "33", "100", "56"},
		},
		{
			name:     "merge negative number -1,000",
			line:     []string{"label", "-1", "000", "385", "77", "896", "33", "100", "56", "2,428", "3,324"},
			expected: 10,
			want:     []string{"label", "-1,000", "385", "77", "896", "33", "100", "56", "2,428", "3,324"},
		},
		{
			name:     "merge already-merged comma number extending",
			line:     []string{"label", "1,000", "000", "385", "77", "896", "33", "100", "56", "2,428"},
			expected: 9,
			want:     []string{"label", "1,000,000", "385", "77", "896", "33", "100", "56", "2,428"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mergeCommaSplitNumbers(tt.line, tt.expected)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got  %v\nwant %v", got, tt.want)
			}
		})
	}
}

func TestLooksLikeCommaSplit(t *testing.T) {
	tests := []struct {
		left, right string
		want        bool
	}{
		{"1", "000", true},
		{"12", "345", true},
		{"-1", "000", true},
		{"1,000", "000", true}, // already has comma, adding another group
		{"434", "385", false},  // 3-digit left is ambiguous with standalone column values
		{"", "000", false},
		{"abc", "000", false},
		{"1", "00", false},  // right not 3 digits
		{"1", "0000", false}, // right not 3 digits
		{"1", "abc", false},
		{"1%", "000", false}, // left doesn't end with digit
	}
	for _, tt := range tests {
		got := looksLikeCommaSplit(tt.left, tt.right)
		if got != tt.want {
			t.Errorf("looksLikeCommaSplit(%q, %q) = %v, want %v", tt.left, tt.right, got, tt.want)
		}
	}
}

func TestParsePagePDF(t *testing.T) {
	pages, err := ExtractContentStreams("testdata/page.pdf")
	if err != nil {
		t.Fatalf("ExtractContentStreams: %v", err)
	}
	if len(pages) != 1 {
		t.Fatalf("expected 1 page, got %d", len(pages))
	}

	items := ExtractTextItems(pages[0])
	stats, err := ParsePage(items)
	if err != nil {
		t.Fatalf("ParsePage: %v", err)
	}

	// Header.
	assertEqual(t, "County", stats.County, "ATLANTIC")
	assertEqual(t, "Municipality", stats.Municipality, "ABSECON")
	assertEqual(t, "DateRange", stats.DateRange, "JULY 2023 - JUNE 2024")

	// Filings - Prior Period.
	assertEqual(t, "Filings.Prior.Label", stats.Filings.PriorPeriod.Label, "Jul 2022 - Jun 2023")
	assertEqual(t, "Filings.Prior.Indictables", stats.Filings.PriorPeriod.Indictables, "434")
	assertEqual(t, "Filings.Prior.DPAndPDP", stats.Filings.PriorPeriod.DPAndPDP, "385")
	assertEqual(t, "Filings.Prior.OtherCriminal", stats.Filings.PriorPeriod.OtherCriminal, "77")
	assertEqual(t, "Filings.Prior.CriminalTotal", stats.Filings.PriorPeriod.CriminalTotal, "896")
	assertEqual(t, "Filings.Prior.DWI", stats.Filings.PriorPeriod.DWI, "33")
	assertEqual(t, "Filings.Prior.TrafficMoving", stats.Filings.PriorPeriod.TrafficMoving, "2,339")
	assertEqual(t, "Filings.Prior.Parking", stats.Filings.PriorPeriod.Parking, "56")
	assertEqual(t, "Filings.Prior.TrafficTotal", stats.Filings.PriorPeriod.TrafficTotal, "2,428")
	assertEqual(t, "Filings.Prior.GrandTotal", stats.Filings.PriorPeriod.GrandTotal, "3,324")

	// Filings - Current Period.
	assertEqual(t, "Filings.Current.Indictables", stats.Filings.CurrentPeriod.Indictables, "232")
	assertEqual(t, "Filings.Current.GrandTotal", stats.Filings.CurrentPeriod.GrandTotal, "3,314")

	// Filings - % Change.
	assertEqual(t, "Filings.PctChange.Label", stats.Filings.PctChange.Label, "% Change")
	assertEqual(t, "Filings.PctChange.Indictables", stats.Filings.PctChange.Indictables, "-47%")
	assertEqual(t, "Filings.PctChange.GrandTotal", stats.Filings.PctChange.GrandTotal, "0%")

	// Resolutions.
	assertEqual(t, "Resolutions.Prior.Indictables", stats.Resolutions.PriorPeriod.Indictables, "439")
	assertEqual(t, "Resolutions.PctChange.CriminalTotal", stats.Resolutions.PctChange.CriminalTotal, "-25%")

	// Clearance (2-row section).
	assertEqual(t, "Clearance.Prior.TrafficMoving", stats.Clearance.PriorPeriod.TrafficMoving, "-120")
	assertEqual(t, "Clearance.Current.GrandTotal", stats.Clearance.CurrentPeriod.GrandTotal, "-220")

	// Clearance Percent.
	assertEqual(t, "ClearancePct.Prior.Indictables", stats.ClearancePct.PriorPeriod.Indictables, "101%")
	assertEqual(t, "ClearancePct.Current.Parking", stats.ClearancePct.CurrentPeriod.Parking, "235%")

	// Backlog (tests kerning concatenation: (8)0(8) → "88", (2)0(3) → "23").
	assertEqual(t, "Backlog.Prior.Label", stats.Backlog.PriorPeriod.Label, "Jun 2023")
	assertEqual(t, "Backlog.Prior.Indictables", stats.Backlog.PriorPeriod.Indictables, "0")
	assertEqual(t, "Backlog.Prior.DPAndPDP", stats.Backlog.PriorPeriod.DPAndPDP, "88")
	assertEqual(t, "Backlog.Prior.OtherCriminal", stats.Backlog.PriorPeriod.OtherCriminal, "23")
	assertEqual(t, "Backlog.Prior.CriminalTotal", stats.Backlog.PriorPeriod.CriminalTotal, "111")
	assertEqual(t, "Backlog.Prior.GrandTotal", stats.Backlog.PriorPeriod.GrandTotal, "318")
	assertEqual(t, "Backlog.Current.DPAndPDP", stats.Backlog.CurrentPeriod.DPAndPDP, "68")
	assertEqual(t, "Backlog.PctChange.Indictables", stats.Backlog.PctChange.Indictables, "- -")

	// Backlog/100 Mthly Filings.
	assertEqual(t, "BacklogPer100.Prior.DPAndPDP", stats.BacklogPer100.PriorPeriod.DPAndPDP, "274")
	assertEqual(t, "BacklogPer100.Prior.Parking", stats.BacklogPer100.PriorPeriod.Parking, "579")
	assertEqual(t, "BacklogPer100.PctChange.Parking", stats.BacklogPer100.PctChange.Parking, "-84%")

	// Backlog Percent.
	assertEqual(t, "BacklogPct.Prior.DPAndPDP", stats.BacklogPct.PriorPeriod.DPAndPDP, "77%")
	assertEqual(t, "BacklogPct.Current.GrandTotal", stats.BacklogPct.CurrentPeriod.GrandTotal, "40%")

	// Active Pending (tests kerning: (9)0(6) → "96", (2)0(5) → "25").
	assertEqual(t, "ActivePending.Prior.Indictables", stats.ActivePending.PriorPeriod.Indictables, "0")
	assertEqual(t, "ActivePending.Prior.DPAndPDP", stats.ActivePending.PriorPeriod.DPAndPDP, "115")
	assertEqual(t, "ActivePending.Current.DPAndPDP", stats.ActivePending.CurrentPeriod.DPAndPDP, "96")
	assertEqual(t, "ActivePending.Current.OtherCriminal", stats.ActivePending.CurrentPeriod.OtherCriminal, "25")
	assertEqual(t, "ActivePending.Current.CriminalTotal", stats.ActivePending.CurrentPeriod.CriminalTotal, "121")
	assertEqual(t, "ActivePending.Current.GrandTotal", stats.ActivePending.CurrentPeriod.GrandTotal, "777")
	assertEqual(t, "ActivePending.PctChange.DWI", stats.ActivePending.PctChange.DWI, "90%")
	assertEqual(t, "ActivePending.PctChange.GrandTotal", stats.ActivePending.PctChange.GrandTotal, "22%")
}

func TestCoverPageSkipped(t *testing.T) {
	pages, err := ExtractContentStreams("testdata/cover.pdf")
	if err != nil {
		t.Fatalf("ExtractContentStreams: %v", err)
	}
	// The cover page is now returned (no longer filtered in ExtractContentStreams),
	// but ContainsFilings should correctly identify it as a non-data page.
	for i, page := range pages {
		items := ExtractTextItems(page)
		if ContainsFilings(items) {
			t.Errorf("page %d: expected cover page to not contain Filings", i)
		}
	}
}

func assertEqual(t *testing.T, field, got, want string) {
	t.Helper()
	if got != want {
		t.Errorf("%s: got %q, want %q", field, got, want)
	}
}
