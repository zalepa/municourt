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

// Parse implements the "parse" subcommand: read a PDF (or directory of PDFs),
// extract municipal court statistics, and write JSON + CSV output files.
func Parse(args []string) {
	fs := flag.NewFlagSet("parse", flag.ExitOnError)
	jsonOut := fs.String("json", "", "output JSON file path (single file mode only)")
	csvOut := fs.String("csv", "", "output CSV file path (single file mode only)")
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: municourt parse <input.pdf | directory> [--json output.json] [--csv output.csv]\n\n")
		fmt.Fprintf(os.Stderr, "If a directory is given, all *.pdf files in it are parsed and output\nfiles are written alongside each PDF.\n\n")
		fs.PrintDefaults()
	}
	fs.Parse(args)

	if fs.NArg() < 1 {
		fs.Usage()
		os.Exit(1)
	}

	inputPath := fs.Arg(0)

	info, err := os.Stat(inputPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	if info.IsDir() {
		pdfs, err := filepath.Glob(filepath.Join(inputPath, "*.pdf"))
		if err != nil {
			fmt.Fprintf(os.Stderr, "error globbing directory: %v\n", err)
			os.Exit(1)
		}
		if len(pdfs) == 0 {
			fmt.Fprintf(os.Stderr, "no PDF files found in %s\n", inputPath)
			os.Exit(1)
		}
		for _, pdf := range pdfs {
			parseSinglePDF(pdf, "", "")
		}
	} else {
		// Default output paths: same directory and base name as input.
		dir := filepath.Dir(inputPath)
		base := strings.TrimSuffix(filepath.Base(inputPath), filepath.Ext(inputPath))
		if *jsonOut == "" {
			*jsonOut = filepath.Join(dir, base+".json")
		}
		if *csvOut == "" {
			*csvOut = filepath.Join(dir, base+".csv")
		}
		parseSinglePDF(inputPath, *jsonOut, *csvOut)
	}
}

func parseSinglePDF(inputPath, jsonOut, csvOut string) {
	dir := filepath.Dir(inputPath)
	base := strings.TrimSuffix(filepath.Base(inputPath), filepath.Ext(inputPath))
	if jsonOut == "" {
		jsonOut = filepath.Join(dir, base+".json")
	}
	if csvOut == "" {
		csvOut = filepath.Join(dir, base+".csv")
	}

	pages, err := parser.ExtractContentStreams(inputPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: error extracting PDF streams: %v\n", filepath.Base(inputPath), err)
		return
	}

	var results []parser.MunicipalityStats
	var errors []string

	for i, page := range pages {
		items := parser.ExtractTextItems(page)
		if !parser.ContainsFilings(items) {
			continue
		}
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
		fmt.Fprintf(os.Stderr, "%s: error marshaling JSON: %v\n", filepath.Base(inputPath), err)
		return
	}
	if err := os.WriteFile(jsonOut, jsonData, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "%s: error writing JSON: %v\n", filepath.Base(inputPath), err)
		return
	}

	// Write CSV.
	if err := writeCSV(csvOut, results); err != nil {
		fmt.Fprintf(os.Stderr, "%s: error writing CSV: %v\n", filepath.Base(inputPath), err)
		return
	}

	// Summary.
	fmt.Fprintf(os.Stderr, "%s: %d pages, %d successful, %d errors → %s\n",
		filepath.Base(inputPath), len(pages), len(results), len(errors), filepath.Base(jsonOut))
	for _, e := range errors {
		fmt.Fprintf(os.Stderr, "  %s\n", e)
	}
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
