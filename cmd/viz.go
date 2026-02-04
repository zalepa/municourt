package cmd

import (
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/zalepa/municourt/parser"
)

type timeRecord struct {
	date  string
	stats []parser.MunicipalityStats
}

type dataPoint struct {
	date  string
	value float64
}

var validMetrics = []string{
	"filings", "resolutions", "clearance", "clearance-pct",
	"backlog", "backlog-per-100", "backlog-pct", "active-pending",
}

var validTypes = []string{
	"grand-total", "indictables", "dp-pdp", "other-criminal",
	"criminal-total", "dwi", "traffic-moving", "parking", "traffic-total",
}

var rateMetrics = map[string]bool{
	"clearance-pct": true,
	"backlog-pct":   true,
	"backlog-per-100": true,
}

// Viz implements the "viz" subcommand.
func Viz(args []string) {
	fs := flag.NewFlagSet("viz", flag.ExitOnError)
	dir := fs.String("dir", ".", "directory containing parsed JSON files")
	level := fs.String("level", "county", "aggregation level: state, county, municipality")
	metric := fs.String("metric", "filings", "metric to display")
	caseType := fs.String("type", "grand-total", "case type column")
	county := fs.String("county", "", "county filter")
	municipality := fs.String("municipality", "", "municipality filter")
	pdfOut := fs.String("pdf", "", "output PDF file path (omit for terminal output)")

	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, `Usage: municourt viz [dir] [flags]

Visualize municipal court statistics over time.

Flags:
`)
		fs.PrintDefaults()
		fmt.Fprintf(os.Stderr, `
Metrics: %s
Types:   %s

Examples:
  municourt viz ./parsed --level state --metric filings
  municourt viz ./parsed --level county --pdf county.pdf
  municourt viz --dir ./parsed --level county --county ATLANTIC
  municourt viz --dir ./parsed --level municipality --county ATLANTIC
`, strings.Join(validMetrics, ", "), strings.Join(validTypes, ", "))
	}
	// Reorder args so the first positional arg (dir) comes after all flags.
	// Go's flag package stops parsing at the first non-flag argument.
	args = reorderArgs(args)
	fs.Parse(args)

	if fs.NArg() > 0 {
		*dir = fs.Arg(0)
	}

	if !contains(validMetrics, *metric) {
		fmt.Fprintf(os.Stderr, "invalid --metric %q; valid options: %s\n", *metric, strings.Join(validMetrics, ", "))
		os.Exit(1)
	}
	if !contains(validTypes, *caseType) {
		fmt.Fprintf(os.Stderr, "invalid --type %q; valid options: %s\n", *caseType, strings.Join(validTypes, ", "))
		os.Exit(1)
	}
	if *level != "state" && *level != "county" && *level != "municipality" {
		fmt.Fprintf(os.Stderr, "invalid --level %q; valid options: state, county, municipality\n", *level)
		os.Exit(1)
	}

	*county = strings.ToUpper(*county)
	*municipality = strings.ToUpper(*municipality)

	records, err := loadRecords(*dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error loading data: %v\n", err)
		os.Exit(1)
	}
	if len(records) == 0 {
		fmt.Fprintf(os.Stderr, "no JSON files found in %s\n", *dir)
		os.Exit(1)
	}

	series, dates := buildSeries(records, *metric, *caseType, *level, *county, *municipality)
	if len(series) == 0 {
		fmt.Fprintf(os.Stderr, "no data matched the given filters\n")
		os.Exit(1)
	}

	title := metricLabel(*metric) + " — " + typeLabel(*caseType)

	// Determine display mode: single entity → line chart, multiple → sparkline table.
	singleEntity := false
	switch *level {
	case "state":
		singleEntity = true
	case "county":
		singleEntity = *county != ""
	case "municipality":
		singleEntity = *municipality != ""
	}

	if *pdfOut != "" {
		sortedDates := sortDates(dates)
		if err := renderPDF(*pdfOut, title, series, sortedDates, *level == "county", singleEntity); err != nil {
			fmt.Fprintf(os.Stderr, "error writing PDF: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("wrote %s\n", *pdfOut)
		return
	}

	if singleEntity {
		// Get the single entity name.
		var name string
		var points []dataPoint
		for k, v := range series {
			name = k
			points = v
			break
		}
		renderChart(title+" — "+name, points)
	} else {
		renderTable(title, series, dates, *level == "county")
	}
}

var datePattern = regexp.MustCompile(`(\d{4})-(\d{2})`)

func loadRecords(dir string) ([]timeRecord, error) {
	matches, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		return nil, err
	}

	var records []timeRecord
	for _, path := range matches {
		base := filepath.Base(path)
		m := datePattern.FindStringSubmatch(base)
		if m == nil {
			continue
		}
		date := m[1] + "-" + m[2]

		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", path, err)
		}
		var stats []parser.MunicipalityStats
		if err := json.Unmarshal(data, &stats); err != nil {
			return nil, fmt.Errorf("parsing %s: %w", path, err)
		}
		records = append(records, timeRecord{date: date, stats: stats})
	}

	sort.Slice(records, func(i, j int) bool {
		return records[i].date < records[j].date
	})
	return records, nil
}

func buildSeries(records []timeRecord, metric, caseType, level, county, municipality string) (map[string][]dataPoint, map[string]bool) {
	// For each time period, aggregate values by entity.
	type accumulator struct {
		sum   float64
		count int
	}
	isRate := rateMetrics[metric]

	series := make(map[string][]dataPoint)
	allDates := make(map[string]bool)

	for _, rec := range records {
		allDates[rec.date] = true
		accum := make(map[string]*accumulator)

		for _, s := range rec.stats {
			key := entityKey(s, level, county, municipality)
			if key == "" {
				continue
			}
			row := getRow(s, metric)
			val := getField(row, caseType)
			if math.IsNaN(val) {
				continue
			}
			a, ok := accum[key]
			if !ok {
				a = &accumulator{}
				accum[key] = a
			}
			a.sum += val
			a.count++
		}

		for key, a := range accum {
			var val float64
			if isRate {
				val = a.sum / float64(a.count)
			} else {
				val = a.sum
			}
			series[key] = append(series[key], dataPoint{date: rec.date, value: val})
		}
	}

	return series, allDates
}

func entityKey(s parser.MunicipalityStats, level, countyFilter, muniFilter string) string {
	switch level {
	case "state":
		return "STATEWIDE"
	case "county":
		if countyFilter != "" && strings.ToUpper(s.County) != countyFilter {
			return ""
		}
		return strings.ToUpper(s.County)
	case "municipality":
		upperCounty := strings.ToUpper(s.County)
		upperMuni := strings.ToUpper(s.Municipality)
		if countyFilter != "" && upperCounty != countyFilter {
			return ""
		}
		if muniFilter != "" && upperMuni != muniFilter {
			return ""
		}
		return upperMuni
	}
	return ""
}

func getRow(s parser.MunicipalityStats, metric string) parser.RowData {
	switch metric {
	case "filings":
		return s.Filings.CurrentPeriod
	case "resolutions":
		return s.Resolutions.CurrentPeriod
	case "clearance":
		return s.Clearance.CurrentPeriod
	case "clearance-pct":
		return s.ClearancePct.CurrentPeriod
	case "backlog":
		return s.Backlog.CurrentPeriod
	case "backlog-per-100":
		return s.BacklogPer100.CurrentPeriod
	case "backlog-pct":
		return s.BacklogPct.CurrentPeriod
	case "active-pending":
		return s.ActivePending.CurrentPeriod
	}
	return parser.RowData{}
}

func getField(r parser.RowData, caseType string) float64 {
	var s string
	switch caseType {
	case "grand-total":
		s = r.GrandTotal
	case "indictables":
		s = r.Indictables
	case "dp-pdp":
		s = r.DPAndPDP
	case "other-criminal":
		s = r.OtherCriminal
	case "criminal-total":
		s = r.CriminalTotal
	case "dwi":
		s = r.DWI
	case "traffic-moving":
		s = r.TrafficMoving
	case "parking":
		s = r.Parking
	case "traffic-total":
		s = r.TrafficTotal
	}
	return parseNumber(s)
}

func parseNumber(s string) float64 {
	s = strings.TrimSpace(s)
	if s == "" || s == "- -" || s == "--" {
		return math.NaN()
	}
	s = strings.ReplaceAll(s, ",", "")
	s = strings.TrimSuffix(s, "%")
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return math.NaN()
	}
	return v
}

func renderTable(title string, series map[string][]dataPoint, dates map[string]bool, includeStatewide bool) {
	// Sort dates for header.
	sortedDates := make([]string, 0, len(dates))
	for d := range dates {
		sortedDates = append(sortedDates, d)
	}
	sort.Strings(sortedDates)

	// Sort entity names.
	names := make([]string, 0, len(series))
	for k := range series {
		names = append(names, k)
	}
	sort.Strings(names)

	// If county level, compute statewide aggregate and move it to end.
	var statewidePoints []dataPoint
	if includeStatewide && len(names) > 1 {
		stateAgg := make(map[string]float64)
		for _, pts := range series {
			for _, p := range pts {
				stateAgg[p.date] += p.value
			}
		}
		for _, d := range sortedDates {
			if v, ok := stateAgg[d]; ok {
				statewidePoints = append(statewidePoints, dataPoint{date: d, value: v})
			}
		}
	}

	// Find max name length.
	maxName := 0
	for _, n := range names {
		if len(n) > maxName {
			maxName = len(n)
		}
	}
	if includeStatewide && len("STATEWIDE") > maxName {
		maxName = len("STATEWIDE")
	}
	if maxName < 10 {
		maxName = 10
	}

	nPeriods := len(sortedDates)
	dateRange := ""
	if nPeriods > 0 {
		dateRange = fmt.Sprintf("%s to %s (%d periods)", sortedDates[0], sortedDates[nPeriods-1], nPeriods)
	}

	fmt.Println(title)
	fmt.Printf("Trend: %s\n\n", dateRange)

	headerFmt := fmt.Sprintf("%%-%ds  %%10s   %%s", maxName)
	fmt.Printf(headerFmt+"\n", "Entity", "Latest", "Trend")
	fmt.Println(strings.Repeat("─", maxName+2+10+3+nPeriods))

	rowFmt := fmt.Sprintf("%%-%ds  %%10s   %%s", maxName)
	for _, name := range names {
		pts := series[name]
		vals := alignValues(pts, sortedDates)
		latest := lastNonNaN(vals)
		fmt.Printf(rowFmt+"\n", name, formatNum(latest), sparkline(vals))
	}

	if includeStatewide && len(statewidePoints) > 0 {
		fmt.Println(strings.Repeat("─", maxName+2+10+3+nPeriods))
		vals := alignValues(statewidePoints, sortedDates)
		latest := lastNonNaN(vals)
		fmt.Printf(rowFmt+"\n", "STATEWIDE", formatNum(latest), sparkline(vals))
	}
}

// alignValues maps dataPoints to a slice aligned with sortedDates, filling gaps with NaN.
func alignValues(pts []dataPoint, sortedDates []string) []float64 {
	lookup := make(map[string]float64, len(pts))
	for _, p := range pts {
		lookup[p.date] = p.value
	}
	vals := make([]float64, len(sortedDates))
	for i, d := range sortedDates {
		if v, ok := lookup[d]; ok {
			vals[i] = v
		} else {
			vals[i] = math.NaN()
		}
	}
	return vals
}

func lastNonNaN(vals []float64) float64 {
	for i := len(vals) - 1; i >= 0; i-- {
		if !math.IsNaN(vals[i]) {
			return vals[i]
		}
	}
	return math.NaN()
}

func sparkline(values []float64) string {
	blocks := []rune("▁▂▃▄▅▆▇█")
	n := len(blocks)

	// Find min/max ignoring NaN.
	min, max := math.Inf(1), math.Inf(-1)
	for _, v := range values {
		if math.IsNaN(v) {
			continue
		}
		if v < min {
			min = v
		}
		if v > max {
			max = v
		}
	}
	if math.IsInf(min, 1) {
		return strings.Repeat(" ", len(values))
	}

	spread := max - min
	var sb strings.Builder
	for _, v := range values {
		if math.IsNaN(v) {
			sb.WriteRune(' ')
			continue
		}
		idx := 0
		if spread > 0 {
			idx = int((v - min) / spread * float64(n-1))
			if idx >= n {
				idx = n - 1
			}
		} else {
			idx = n / 2
		}
		sb.WriteRune(blocks[idx])
	}
	return sb.String()
}

func renderChart(title string, points []dataPoint) {
	if len(points) == 0 {
		fmt.Println(title)
		fmt.Println("(no data)")
		return
	}

	// Sort points by date.
	sort.Slice(points, func(i, j int) bool {
		return points[i].date < points[j].date
	})

	// Filter out NaN points.
	var filtered []dataPoint
	for _, p := range points {
		if !math.IsNaN(p.value) {
			filtered = append(filtered, p)
		}
	}
	if len(filtered) == 0 {
		fmt.Println(title)
		fmt.Println("(no data)")
		return
	}
	points = filtered

	fmt.Println(title)
	fmt.Println()

	height := 15
	nPoints := len(points)

	// Determine column width: try to fit in ~100 chars for the data area.
	labelWidth := 10 // y-axis label area
	available := 100 - labelWidth
	colWidth := available / nPoints
	if colWidth > 8 {
		colWidth = 8
	}
	if colWidth < 3 {
		colWidth = 3
	}

	// Find value range.
	minVal, maxVal := points[0].value, points[0].value
	for _, p := range points {
		if p.value < minVal {
			minVal = p.value
		}
		if p.value > maxVal {
			maxVal = p.value
		}
	}
	// Add small padding to range.
	valRange := maxVal - minVal
	if valRange == 0 {
		valRange = 1
		minVal -= 0.5
		maxVal += 0.5
	}

	// Map each point to a row (0 = bottom, height-1 = top).
	pointRows := make([]int, nPoints)
	for i, p := range points {
		row := int(math.Round((p.value - minVal) / valRange * float64(height-1)))
		if row < 0 {
			row = 0
		}
		if row >= height {
			row = height - 1
		}
		pointRows[i] = row
	}

	// Build grid.
	totalWidth := nPoints * colWidth
	grid := make([][]rune, height)
	for r := 0; r < height; r++ {
		grid[r] = make([]rune, totalWidth)
		for c := range grid[r] {
			grid[r][c] = ' '
		}
	}

	// Place data points and connecting dots.
	for i := 0; i < nPoints; i++ {
		col := i*colWidth + colWidth/2
		grid[pointRows[i]][col] = '●'

		// Connect to next point with · via linear interpolation.
		if i < nPoints-1 {
			startCol := col
			endCol := (i+1)*colWidth + colWidth/2
			startRow := pointRows[i]
			endRow := pointRows[i+1]
			colSpan := endCol - startCol
			for c := startCol + 1; c < endCol; c++ {
				t := float64(c-startCol) / float64(colSpan)
				r := int(math.Round(float64(startRow) + t*float64(endRow-startRow)))
				if r < 0 {
					r = 0
				}
				if r >= height {
					r = height - 1
				}
				if grid[r][c] == ' ' {
					grid[r][c] = '·'
				}
			}
		}
	}

	// Y-axis labels: 5 evenly spaced.
	yLabels := make(map[int]string)
	for i := 0; i < 5; i++ {
		row := int(math.Round(float64(i) / 4.0 * float64(height-1)))
		val := minVal + float64(row)/float64(height-1)*valRange
		yLabels[row] = formatCompact(val)
	}

	// Render rows top to bottom.
	for r := height - 1; r >= 0; r-- {
		label := ""
		if l, ok := yLabels[r]; ok {
			label = l
		}
		fmt.Printf("%8s │%s\n", label, string(grid[r]))
	}

	// X-axis line.
	fmt.Printf("%8s └%s\n", "", strings.Repeat("─", totalWidth))

	// X-axis labels.
	// Determine how many labels fit.
	labelEvery := 1
	if colWidth < 8 {
		labelEvery = (8 + colWidth - 1) / colWidth
	}
	xLine := make([]byte, totalWidth)
	for i := range xLine {
		xLine[i] = ' '
	}
	for i := 0; i < nPoints; i += labelEvery {
		pos := i*colWidth + colWidth/2 - len(points[i].date)/2
		if pos < 0 {
			pos = 0
		}
		label := points[i].date
		for j := 0; j < len(label) && pos+j < totalWidth; j++ {
			xLine[pos+j] = label[j]
		}
	}
	fmt.Printf("%8s  %s\n", "", string(xLine))
}

func formatNum(v float64) string {
	if math.IsNaN(v) {
		return "- -"
	}
	if v == float64(int64(v)) && math.Abs(v) < 1e15 {
		return formatInt(int64(v))
	}
	return strconv.FormatFloat(v, 'f', 1, 64)
}

func formatInt(v int64) string {
	s := strconv.FormatInt(v, 10)
	if v < 0 {
		return "-" + addCommas(s[1:])
	}
	return addCommas(s)
}

func addCommas(s string) string {
	n := len(s)
	if n <= 3 {
		return s
	}
	var sb strings.Builder
	pre := n % 3
	if pre > 0 {
		sb.WriteString(s[:pre])
		if pre < n {
			sb.WriteByte(',')
		}
	}
	for i := pre; i < n; i += 3 {
		sb.WriteString(s[i : i+3])
		if i+3 < n {
			sb.WriteByte(',')
		}
	}
	return sb.String()
}

func formatCompact(v float64) string {
	abs := math.Abs(v)
	switch {
	case abs >= 1e6:
		return strconv.FormatFloat(v/1e6, 'f', 1, 64) + "M"
	case abs >= 1e3:
		return strconv.FormatFloat(v/1e3, 'f', 0, 64) + "k"
	default:
		return strconv.FormatFloat(v, 'f', 0, 64)
	}
}

func metricLabel(m string) string {
	labels := map[string]string{
		"filings":        "Filings",
		"resolutions":    "Resolutions",
		"clearance":      "Clearance",
		"clearance-pct":  "Clearance %",
		"backlog":        "Backlog",
		"backlog-per-100": "Backlog per 100",
		"backlog-pct":    "Backlog %",
		"active-pending": "Active Pending",
	}
	return labels[m]
}

func typeLabel(t string) string {
	labels := map[string]string{
		"grand-total":    "Grand Total",
		"indictables":    "Indictables",
		"dp-pdp":         "DP & PDP",
		"other-criminal": "Other Criminal",
		"criminal-total": "Criminal Total",
		"dwi":            "DWI",
		"traffic-moving": "Traffic Moving",
		"parking":        "Parking",
		"traffic-total":  "Traffic Total",
	}
	return labels[t]
}

// reorderArgs moves positional arguments to the end so that Go's flag package
// can parse all flags regardless of where a positional dir argument appears.
func reorderArgs(args []string) []string {
	var flags, positional []string
	for i := 0; i < len(args); i++ {
		if args[i] == "--" {
			positional = append(positional, args[i+1:]...)
			break
		}
		if strings.HasPrefix(args[i], "-") {
			flags = append(flags, args[i])
			// Consume the next arg as the flag's value unless it looks like a flag itself.
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") && !strings.Contains(args[i], "=") {
				flags = append(flags, args[i+1])
				i++
			}
		} else {
			positional = append(positional, args[i])
		}
	}
	return append(flags, positional...)
}

func sortDates(dates map[string]bool) []string {
	sorted := make([]string, 0, len(dates))
	for d := range dates {
		sorted = append(sorted, d)
	}
	sort.Strings(sorted)
	return sorted
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
