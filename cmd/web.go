package cmd

import (
	"embed"
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"net/http"
	"os"
	"sort"
	"strings"
)

//go:embed web.html
var htmlContent embed.FS

type metadata struct {
	Counties       []string                `json:"counties"`
	Municipalities map[string][]string     `json:"municipalities"`
	Metrics        []labelValue            `json:"metrics"`
	Types          []labelValue            `json:"types"`
}

type labelValue struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

type seriesResponse struct {
	Title  string       `json:"title"`
	Dates  []string     `json:"dates"`
	Series []seriesData `json:"series"`
}

type seriesData struct {
	Name   string     `json:"name"`
	Values []*float64 `json:"values"`
}

// Web implements the "web" subcommand.
func Web(args []string) {
	fs := flag.NewFlagSet("web", flag.ExitOnError)
	dir := fs.String("dir", ".", "directory containing parsed JSON files")
	port := fs.String("port", "8080", "HTTP server port")

	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: municourt web [dir] [--port 8080]\n\nStart an interactive web dashboard.\n\nFlags:\n")
		fs.PrintDefaults()
	}
	args = reorderArgs(args)
	fs.Parse(args)

	if fs.NArg() > 0 {
		*dir = fs.Arg(0)
	}

	records, err := loadRecords(*dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error loading data: %v\n", err)
		os.Exit(1)
	}
	if len(records) == 0 {
		fmt.Fprintf(os.Stderr, "warning: no JSON files found in %s, starting with empty data\n", *dir)
	}

	meta := buildMetadata(records)
	metaJSON, _ := json.Marshal(meta)

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		data, _ := htmlContent.ReadFile("web.html")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(data)
	})

	http.HandleFunc("/api/metadata", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(metaJSON)
	})

	http.HandleFunc("/api/series", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		level := q.Get("level")
		metric := q.Get("metric")
		caseType := q.Get("type")
		county := strings.ToUpper(q.Get("county"))
		municipality := strings.ToUpper(q.Get("municipality"))

		if !contains(validMetrics, metric) {
			metric = "filings"
		}
		if !contains(validTypes, caseType) {
			caseType = "grand-total"
		}
		if level != "state" && level != "county" && level != "municipality" {
			level = "county"
		}

		series, dates := buildSeries(records, metric, caseType, level, county, municipality)
		sortedDates := sortDates(dates)
		title := metricLabel(metric) + " — " + typeLabel(caseType)

		resp := seriesResponse{
			Title: title,
			Dates: sortedDates,
		}

		// Sort series names for stable ordering.
		names := make([]string, 0, len(series))
		for k := range series {
			names = append(names, k)
		}
		sort.Strings(names)

		for _, name := range names {
			pts := series[name]
			aligned := alignValues(pts, sortedDates)
			values := make([]*float64, len(aligned))
			for i, v := range aligned {
				if math.IsNaN(v) {
					values[i] = nil
				} else {
					f := v
					values[i] = &f
				}
			}
			resp.Series = append(resp.Series, seriesData{Name: name, Values: values})
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})

	addr := ":" + *port
	fmt.Printf("serving on http://localhost%s\n", addr)
	if err := http.ListenAndServe(addr, nil); err != nil {
		fmt.Fprintf(os.Stderr, "server error: %v\n", err)
		os.Exit(1)
	}
}

func buildMetadata(records []timeRecord) metadata {
	countySet := make(map[string]bool)
	muniMap := make(map[string]map[string]bool)

	for _, rec := range records {
		for _, s := range rec.stats {
			c := strings.ToUpper(s.County)
			countySet[c] = true
			if _, ok := muniMap[c]; !ok {
				muniMap[c] = make(map[string]bool)
			}
			muniMap[c][strings.ToUpper(s.Municipality)] = true
		}
	}

	counties := make([]string, 0, len(countySet))
	for c := range countySet {
		counties = append(counties, c)
	}
	sort.Strings(counties)

	municipalities := make(map[string][]string, len(muniMap))
	for c, ms := range muniMap {
		munis := make([]string, 0, len(ms))
		for m := range ms {
			munis = append(munis, m)
		}
		sort.Strings(munis)
		municipalities[c] = munis
	}

	metrics := make([]labelValue, len(validMetrics))
	for i, m := range validMetrics {
		metrics[i] = labelValue{Value: m, Label: metricLabel(m)}
	}
	types := make([]labelValue, len(validTypes))
	for i, t := range validTypes {
		types[i] = labelValue{Value: t, Label: typeLabel(t)}
	}

	return metadata{
		Counties:       counties,
		Municipalities: municipalities,
		Metrics:        metrics,
		Types:          types,
	}
}
