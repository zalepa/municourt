package cmd

import (
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/zalepa/municourt/parser"
)

// Parse implements the "parse" subcommand: read a PDF, extract municipal court
// statistics, and write JSON + CSV output files.
func Parse(args []string) {
	fs := flag.NewFlagSet("parse", flag.ExitOnError)
	jsonOut := fs.String("json", "", "output JSON file path")
	csvOut := fs.String("csv", "", "output CSV file path")
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: municourt parse <input.pdf> [--json output.json] [--csv output.csv]\n")
		fs.PrintDefaults()
	}
	fs.Parse(args)

	if fs.NArg() < 1 {
		fs.Usage()
		os.Exit(1)
	}

	inputPath := fs.Arg(0)

	// Default output paths: same directory and base name as input.
	dir := filepath.Dir(inputPath)
	base := strings.TrimSuffix(filepath.Base(inputPath), filepath.Ext(inputPath))
	if *jsonOut == "" {
		*jsonOut = filepath.Join(dir, base+".json")
	}
	if *csvOut == "" {
		*csvOut = filepath.Join(dir, base+".csv")
	}

	streams, err := parser.ExtractContentStreams(inputPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error extracting PDF streams: %v\n", err)
		os.Exit(1)
	}

	var results []parser.MunicipalityStats
	var errors []string

	for i, stream := range streams {
		items := parser.ExtractTextItems(stream)
		stats, err := parser.ParsePage(items)
		if err != nil {
			errors = append(errors, fmt.Sprintf("page %d: %v", i+1, err))
			continue
		}
		results = append(results, stats)
	}

	// Write JSON.
	jsonData, err := json.MarshalIndent(results, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error marshaling JSON: %v\n", err)
		os.Exit(1)
	}
	if err := os.WriteFile(*jsonOut, jsonData, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "error writing JSON: %v\n", err)
		os.Exit(1)
	}

	// Write CSV.
	if err := writeCSV(*csvOut, results); err != nil {
		fmt.Fprintf(os.Stderr, "error writing CSV: %v\n", err)
		os.Exit(1)
	}

	// Summary.
	fmt.Fprintf(os.Stderr, "Parsed %d pages, %d successful, %d errors\n",
		len(streams), len(results), len(errors))
	for _, e := range errors {
		fmt.Fprintf(os.Stderr, "  %s\n", e)
	}
	fmt.Fprintf(os.Stderr, "JSON: %s\n", *jsonOut)
	fmt.Fprintf(os.Stderr, "CSV:  %s\n", *csvOut)
}

func writeCSV(path string, stats []parser.MunicipalityStats) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	w := csv.NewWriter(f)
	defer w.Flush()

	// Build header.
	header := []string{"County", "Municipality", "DateRange"}
	sections := []string{
		"Filings_Prior", "Filings_Current", "Filings_PctChange",
		"Resolutions_Prior", "Resolutions_Current", "Resolutions_PctChange",
		"Clearance_Prior", "Clearance_Current",
		"ClearancePct_Prior", "ClearancePct_Current",
		"Backlog_Prior", "Backlog_Current", "Backlog_PctChange",
		"BacklogPer100_Prior", "BacklogPer100_Current", "BacklogPer100_PctChange",
		"BacklogPct_Prior", "BacklogPct_Current",
		"ActivePending_Prior", "ActivePending_Current", "ActivePending_PctChange",
	}
	cols := []string{"Label", "Indictables", "DPAndPDP", "OtherCriminal", "CriminalTotal",
		"DWI", "TrafficMoving", "Parking", "TrafficTotal", "GrandTotal"}

	for _, sec := range sections {
		for _, col := range cols {
			header = append(header, sec+"_"+col)
		}
	}

	if err := w.Write(header); err != nil {
		return err
	}

	for _, s := range stats {
		row := []string{s.County, s.Municipality, s.DateRange}
		allRows := []parser.RowData{
			s.Filings.PriorPeriod, s.Filings.CurrentPeriod, s.Filings.PctChange,
			s.Resolutions.PriorPeriod, s.Resolutions.CurrentPeriod, s.Resolutions.PctChange,
			s.Clearance.PriorPeriod, s.Clearance.CurrentPeriod,
			s.ClearancePct.PriorPeriod, s.ClearancePct.CurrentPeriod,
			s.Backlog.PriorPeriod, s.Backlog.CurrentPeriod, s.Backlog.PctChange,
			s.BacklogPer100.PriorPeriod, s.BacklogPer100.CurrentPeriod, s.BacklogPer100.PctChange,
			s.BacklogPct.PriorPeriod, s.BacklogPct.CurrentPeriod,
			s.ActivePending.PriorPeriod, s.ActivePending.CurrentPeriod, s.ActivePending.PctChange,
		}
		for _, r := range allRows {
			row = append(row, r.Label, r.Indictables, r.DPAndPDP, r.OtherCriminal,
				r.CriminalTotal, r.DWI, r.TrafficMoving, r.Parking, r.TrafficTotal, r.GrandTotal)
		}
		if err := w.Write(row); err != nil {
			return err
		}
	}

	return nil
}
