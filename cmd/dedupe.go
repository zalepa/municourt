package cmd

import (
	"bufio"
	"fmt"
	"os"
	"sort"
	"strings"
)

// municipalSuffixes lists common municipal designation suffixes in NJ. Order
// matters: longer suffixes must come first so "TOWNSHIP" is tried before "TOWN".
var municipalSuffixes = []string{
	"TOWNSHIP", "TOWN", "TWP", "BOROUGH", "BORO", "CITY", "VILLAGE",
}

// stripMunicipalSuffix removes a trailing municipal designation (e.g., "TOWN",
// "TWP", "CITY") from a municipality name. Returns the uppercased base name.
func stripMunicipalSuffix(name string) string {
	upper := strings.TrimSpace(strings.ToUpper(name))
	for _, suffix := range municipalSuffixes {
		if strings.HasSuffix(upper, " "+suffix) {
			return upper[:len(upper)-len(suffix)-1]
		}
	}
	return upper
}

type duplicateCandidate struct {
	county string
	nameA  string   // keeper (more recent data)
	nameB  string   // to be renamed
	datesA []string // sorted YYYY-MM dates
	datesB []string
}

// findDuplicates detects municipality names within the same county that likely
// refer to the same entity. It groups names by their suffix-stripped base, then
// checks whether the two variants ever co-occur in the same time period. If
// they don't overlap, they're flagged as a candidate merge.
func findDuplicates(parsed []parseResult) []duplicateCandidate {
	type nameInfo struct {
		dates map[string]bool
	}
	// county -> strippedName -> actualName -> info
	groups := make(map[string]map[string]map[string]*nameInfo)

	for _, r := range parsed {
		if r.failed || r.date == "" {
			continue
		}
		for _, s := range r.results {
			county := strings.ToUpper(s.County)
			name := strings.ToUpper(s.Municipality)
			stripped := stripMunicipalSuffix(name)

			if groups[county] == nil {
				groups[county] = make(map[string]map[string]*nameInfo)
			}
			if groups[county][stripped] == nil {
				groups[county][stripped] = make(map[string]*nameInfo)
			}
			if groups[county][stripped][name] == nil {
				groups[county][stripped][name] = &nameInfo{dates: make(map[string]bool)}
			}
			groups[county][stripped][name].dates[r.date] = true
		}
	}

	var candidates []duplicateCandidate
	for county, strippedGroups := range groups {
		for _, nameMap := range strippedGroups {
			if len(nameMap) < 2 {
				continue
			}
			names := make([]string, 0, len(nameMap))
			for n := range nameMap {
				names = append(names, n)
			}
			sort.Strings(names)

			for i := 0; i < len(names); i++ {
				for j := i + 1; j < len(names); j++ {
					infoA, infoB := nameMap[names[i]], nameMap[names[j]]

					// If they co-occur in any time period, they're distinct entities.
					hasOverlap := false
					for d := range infoA.dates {
						if infoB.dates[d] {
							hasOverlap = true
							break
						}
					}
					if hasOverlap {
						continue
					}

					datesA := sortedKeys(infoA.dates)
					datesB := sortedKeys(infoB.dates)

					// Keeper: the name with more recent data.
					a, b := names[i], names[j]
					dA, dB := datesA, datesB
					recentA, recentB := datesA[len(datesA)-1], datesB[len(datesB)-1]
					if recentB > recentA {
						a, b = b, a
						dA, dB = dB, dA
					}

					candidates = append(candidates, duplicateCandidate{
						county: county,
						nameA:  a,
						nameB:  b,
						datesA: dA,
						datesB: dB,
					})
				}
			}
		}
	}

	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].county != candidates[j].county {
			return candidates[i].county < candidates[j].county
		}
		return candidates[i].nameA < candidates[j].nameA
	})
	return candidates
}

func sortedKeys(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func formatDateRange(dates []string) string {
	if len(dates) == 0 {
		return "no data"
	}
	if len(dates) == 1 {
		return fmt.Sprintf("%s (1 period)", dates[0])
	}
	return fmt.Sprintf("%s to %s (%d periods)", dates[0], dates[len(dates)-1], len(dates))
}

// deduplicateMunicipalities finds municipality name variants that likely refer
// to the same entity and prompts the user to merge them. Merges are applied
// in-place to the parseResult slice before output files are written.
func deduplicateMunicipalities(parsed []parseResult) {
	candidates := findDuplicates(parsed)
	if len(candidates) == 0 {
		return
	}

	type muniKey struct {
		county, name string
	}
	merges := make(map[muniKey]string)

	scanner := bufio.NewScanner(os.Stdin)
	acceptAll := false
	for _, c := range candidates {
		if acceptAll {
			fmt.Fprintf(os.Stderr, "  %s → %s: %s (%d) + %s (%d)\n",
				c.county, c.nameA, c.nameB, len(c.datesB), c.nameA, len(c.datesA))
			merges[muniKey{c.county, c.nameB}] = c.nameA
			continue
		}

		fmt.Fprintf(os.Stderr, "\nPotential duplicate in %s county:\n", c.county)
		fmt.Fprintf(os.Stderr, "  %-30s %s\n", c.nameA, formatDateRange(c.datesA))
		fmt.Fprintf(os.Stderr, "  %-30s %s\n", c.nameB, formatDateRange(c.datesB))
		fmt.Fprintf(os.Stderr, "Merge %q → %q? [y/N/a(ll)]: ", c.nameB, c.nameA)

		if !scanner.Scan() {
			break
		}
		answer := strings.TrimSpace(strings.ToLower(scanner.Text()))
		switch answer {
		case "a", "all":
			acceptAll = true
			merges[muniKey{c.county, c.nameB}] = c.nameA
		case "y", "yes":
			merges[muniKey{c.county, c.nameB}] = c.nameA
		}
	}

	if len(merges) == 0 {
		return
	}

	applied := 0
	for i := range parsed {
		for j := range parsed[i].results {
			s := &parsed[i].results[j]
			key := muniKey{strings.ToUpper(s.County), strings.ToUpper(s.Municipality)}
			if newName, ok := merges[key]; ok {
				s.Municipality = newName
				applied++
			}
		}
	}
	fmt.Fprintf(os.Stderr, "dedupe: renamed %d entries\n", applied)
}
