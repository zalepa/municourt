package parser

import (
	"fmt"
	"strings"
)

// knownSections lists section names in the order they appear on each page.
var knownSections = []string{
	"Filings",
	"Resolutions",
	"Clearance",
	"Clearance Percent",
	"Backlog",
	"Backlog/100 Mthly Filings",
	"Backlog Percent",
	"Active Pending",
}

// groupIntoLines splits text items into lines using empty-string line-break
// markers. Adjacent empties are collapsed and leading/trailing empties trimmed.
func groupIntoLines(items []string) [][]string {
	var lines [][]string
	var current []string
	for _, item := range items {
		s := strings.TrimSpace(item)
		if s == "" {
			if len(current) > 0 {
				lines = append(lines, current)
				current = nil
			}
		} else {
			current = append(current, s)
		}
	}
	if len(current) > 0 {
		lines = append(lines, current)
	}
	return lines
}

// sectionAliases maps variant section names found in older PDFs to the
// canonical name used in knownSections.
var sectionAliases = map[string]string{
	"Terminations": "Resolutions",
}

// matchSectionName checks if a line represents a known section name.
// Section names may be split across multiple items on the same line
// (e.g., ["Clearance", "Percent"] for "Clearance Percent").
// Comparison ignores spaces so that kerning-induced splits (e.g.,
// "F" + "ilings" for "Filings") don't cause mismatches.
// Aliases (e.g., "Terminations" → "Resolutions") are resolved to the
// canonical name.
func matchSectionName(line []string) string {
	joined := strings.Join(line, " ")
	compact := strings.ReplaceAll(joined, " ", "")
	for _, name := range knownSections {
		if compact == strings.ReplaceAll(name, " ", "") {
			return name
		}
	}
	compactAliasKey := compact
	for alias, canonical := range sectionAliases {
		if compactAliasKey == strings.ReplaceAll(alias, " ", "") {
			return canonical
		}
	}
	return ""
}

// mergeCommaSplitNumbers fixes numbers that were split by large kerning in TJ
// arrays. For example, "1,000" might appear as two items ["1", "000"] when the
// kerning between them exceeds the threshold. This function merges such pairs
// back into single items with commas.
//
// It only activates when a line has more than expectedLen items, to avoid false
// positives on lines that already have the correct count.
//
// Merges are prioritized: pairs where the right part has a leading zero (e.g.,
// "000", "040") are merged first since those can't be standalone values. Then
// pairs with a 1-digit left, then 2-digit left.
func mergeCommaSplitNumbers(line []string, expectedLen int) []string {
	for len(line) > expectedLen {
		bestIdx := -1
		bestPriority := -1

		for i := 0; i < len(line)-1; i++ {
			if !looksLikeCommaSplit(line[i], line[i+1]) {
				continue
			}
			priority := 0
			if line[i+1][0] == '0' {
				priority = 3 // Right has leading zero: can't be standalone.
			} else {
				digits := strings.TrimPrefix(line[i], "-")
				// Strip existing comma groups from already-merged values.
				if idx := strings.LastIndex(digits, ","); idx >= 0 {
					digits = digits[idx+1:]
				}
				if len(digits) == 1 {
					priority = 2
				} else if len(digits) == 2 {
					priority = 1
				}
			}
			if priority > bestPriority {
				bestPriority = priority
				bestIdx = i
			}
		}

		if bestIdx < 0 {
			break
		}

		// Merge the pair at bestIdx.
		merged := line[bestIdx] + "," + line[bestIdx+1]
		newLine := make([]string, 0, len(line)-1)
		newLine = append(newLine, line[:bestIdx]...)
		newLine = append(newLine, merged)
		newLine = append(newLine, line[bestIdx+2:]...)
		line = newLine
	}
	return line
}

// looksLikeCommaSplit returns true if left+right look like two halves of a
// comma-separated number. Right must be exactly 3 digits. Left must be a short
// numeric prefix: either 1-2 digits (optionally negative), or an already-merged
// comma number ending in a 3-digit group. This avoids false positives where two
// separate 3-digit column values (e.g., "434" and "385") sit adjacent.
func looksLikeCommaSplit(left, right string) bool {
	if !isThreeDigits(right) {
		return false
	}
	if left == "" {
		return false
	}
	// Left must end with a digit.
	if last := left[len(left)-1]; last < '0' || last > '9' {
		return false
	}
	// If left already contains a comma, it's been partially merged — allow
	// extending it (e.g., "1,000" + "000" → "1,000,000") as long as the
	// last group is 3 digits.
	if idx := strings.LastIndex(left, ","); idx >= 0 {
		trailing := left[idx+1:]
		return isThreeDigits(trailing)
	}
	// Otherwise, left must be a short numeric prefix: 1-2 digits, optionally
	// with a leading minus sign. 3-digit left values are NOT merged because
	// they're ambiguous with standalone column values.
	stripped := strings.TrimPrefix(left, "-")
	if len(stripped) < 1 || len(stripped) > 2 {
		return false
	}
	for _, c := range stripped {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

func isThreeDigits(s string) bool {
	if len(s) != 3 {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// ParsePage takes the text items extracted from a single page's content stream
// and maps them to a MunicipalityStats struct.
func ParsePage(items []string) (MunicipalityStats, error) {
	lines := groupIntoLines(items)
	pos := 0
	var stats MunicipalityStats

	nextLine := func() ([]string, error) {
		if pos >= len(lines) {
			return nil, fmt.Errorf("unexpected end of lines at line %d", pos)
		}
		l := lines[pos]
		pos++
		return l, nil
	}

	peekLine := func() []string {
		if pos >= len(lines) {
			return nil
		}
		return lines[pos]
	}

	// Header: 4 single-item lines.
	titleLine, err := nextLine()
	if err != nil {
		return stats, fmt.Errorf("reading title: %w", err)
	}
	title := strings.Join(titleLine, " ")
	if !strings.Contains(title, "MUNICIPAL COURT") {
		return stats, fmt.Errorf("expected title containing 'MUNICIPAL COURT', got %q", title)
	}

	dateLine, err := nextLine()
	if err != nil {
		return stats, fmt.Errorf("reading date range: %w", err)
	}
	stats.DateRange = strings.Join(dateLine, " ")

	countyLine, err := nextLine()
	if err != nil {
		return stats, fmt.Errorf("reading county: %w", err)
	}
	stats.County = strings.Join(countyLine, " ")

	muniLine, err := nextLine()
	if err != nil {
		return stats, fmt.Errorf("reading municipality: %w", err)
	}
	stats.Municipality = strings.Join(muniLine, " ")

	// Skip column header lines until we find a section name line.
	for pos < len(lines) {
		if name := matchSectionName(peekLine()); name != "" {
			break
		}
		pos++
	}

	// readRow reads a data row line: label + 9 values.
	readRow := func(sectionName string) (RowData, error) {
		line, err := nextLine()
		if err != nil {
			return RowData{}, fmt.Errorf("section %q: reading data row: %w", sectionName, err)
		}
		line = mergeCommaSplitNumbers(line, 10)
		if len(line) < 1 {
			return RowData{}, fmt.Errorf("section %q: empty data row", sectionName)
		}
		// Pad short rows (e.g., statewide summary pages with fewer columns).
		for len(line) < 10 {
			line = append(line, "- -")
		}
		if len(line) > 10 {
			// Even after merge, too many items. Take first 10 and continue.
			line = line[:10]
		}
		return RowData{
			Label:         line[0],
			Indictables:   line[1],
			DPAndPDP:      line[2],
			OtherCriminal: line[3],
			CriminalTotal: line[4],
			DWI:           line[5],
			TrafficMoving: line[6],
			Parking:       line[7],
			TrafficTotal:  line[8],
			GrandTotal:    line[9],
		}, nil
	}

	readSectionName := func(expected string) error {
		line, err := nextLine()
		if err != nil {
			return fmt.Errorf("reading section name for %q: %w", expected, err)
		}
		got := matchSectionName(line)
		if got == "" {
			got = strings.Join(line, " ")
		}
		if got != expected {
			return fmt.Errorf("expected section %q, got %q", expected, got)
		}
		return nil
	}

	readSectionWithChange := func(name string) (SectionWithChange, error) {
		if err := readSectionName(name); err != nil {
			return SectionWithChange{}, err
		}
		prior, err := readRow(name)
		if err != nil {
			return SectionWithChange{}, err
		}
		current, err := readRow(name)
		if err != nil {
			return SectionWithChange{}, err
		}
		pctChange, err := readRow(name)
		if err != nil {
			return SectionWithChange{}, err
		}
		return SectionWithChange{
			PriorPeriod:   prior,
			CurrentPeriod: current,
			PctChange:     pctChange,
		}, nil
	}

	readSectionTwoRow := func(name string) (SectionTwoRow, error) {
		if err := readSectionName(name); err != nil {
			return SectionTwoRow{}, err
		}
		prior, err := readRow(name)
		if err != nil {
			return SectionTwoRow{}, err
		}
		current, err := readRow(name)
		if err != nil {
			return SectionTwoRow{}, err
		}
		return SectionTwoRow{
			PriorPeriod:   prior,
			CurrentPeriod: current,
		}, nil
	}

	// Sections in order.
	stats.Filings, err = readSectionWithChange("Filings")
	if err != nil {
		return stats, err
	}

	stats.Resolutions, err = readSectionWithChange("Resolutions")
	if err != nil {
		return stats, err
	}

	stats.Clearance, err = readSectionTwoRow("Clearance")
	if err != nil {
		return stats, err
	}

	stats.ClearancePct, err = readSectionTwoRow("Clearance Percent")
	if err != nil {
		return stats, err
	}

	stats.Backlog, err = readSectionWithChange("Backlog")
	if err != nil {
		return stats, err
	}

	stats.BacklogPer100, err = readSectionWithChange("Backlog/100 Mthly Filings")
	if err != nil {
		return stats, err
	}

	stats.BacklogPct, err = readSectionTwoRow("Backlog Percent")
	if err != nil {
		return stats, err
	}

	stats.ActivePending, err = readSectionWithChange("Active Pending")
	if err != nil {
		return stats, err
	}

	return stats, nil
}
