# municourt

A CLI tool that extracts structured data from New Jersey Municipal Court Statistics PDF reports.

Each page of the PDF contains statistics for a single municipality. This tool parses the PDF content streams directly (no OCR), extracting all section data into JSON and CSV formats.

## Usage

```
municourt <input.pdf> [--json output.json] [--csv output.csv]
```

By default, output files are written alongside the input file with the same base name:

```
municourt report.pdf
# writes report.json and report.csv
```

Override output paths:

```
municourt report.pdf --json /tmp/data.json --csv /tmp/data.csv
```

## Output

Each municipality produces a record with:

- **Header**: county, municipality, date range
- **8 sections**, each with sub-rows of 9 column values:

| Section | Sub-rows |
|---|---|
| Filings | Prior, Current, % Change |
| Resolutions | Prior, Current, % Change |
| Clearance | Prior, Current |
| Clearance Percent | Prior, Current |
| Backlog | Prior, Current, % Change |
| Backlog/100 Mthly Filings | Prior, Current, % Change |
| Backlog Percent | Prior, Current |
| Active Pending | Prior, Current, % Change |

Columns: Indictables, D.P. & P.D.P., Other Criminal, Criminal Total, D.W.I., Traffic (moving), Parking, Traffic Total, Grand Total.

Values are stored as strings since they may contain commas, `%`, `- -`, or negative signs.

## Building

```
go build -o municourt .
```

## Testing

```
go test ./...
```

Test fixtures `page.pdf` (ATLANTIC/ABSECON) and `cover.pdf` (cover page) are included in the repo.

## How it works

1. **pdf.go** — Opens the PDF with [pdfcpu](https://github.com/pdfcpu/pdfcpu), iterates pages, decompresses content streams, and skips non-data pages (cover pages).
2. **content.go** — Tokenizes PDF content streams and extracts text from `Tj` and `TJ` operators. Within `TJ` arrays, kerning values determine whether adjacent strings are concatenated (small spacing) or treated as separate columns (large spacing).
3. **parser.go** — Reads the ordered text items and maps them to `MunicipalityStats` structs using the known section layout.
4. **main.go** — CLI that orchestrates the pipeline and writes JSON/CSV output.
