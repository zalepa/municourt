package cmd

import (
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
)

var hrefPattern = regexp.MustCompile(`href="([^"]*munm(\d{4})\.pdf)"`)

// Download implements the "download" subcommand: scrape the NJ Courts
// statistics page for municipal court PDFs and download them.
func Download(args []string) {
	fs := flag.NewFlagSet("download", flag.ExitOnError)
	dir := fs.String("dir", ".", "output directory for downloaded PDFs")
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: municourt download [-dir path]\n")
		fs.PrintDefaults()
	}
	fs.Parse(args)

	if err := os.MkdirAll(*dir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "error creating output directory: %v\n", err)
		os.Exit(1)
	}

	const pageURL = "https://www.njcourts.gov/public/statistics"
	fmt.Fprintf(os.Stderr, "Fetching %s\n", pageURL)

	resp, err := http.Get(pageURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error fetching statistics page: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		fmt.Fprintf(os.Stderr, "unexpected status %d fetching statistics page\n", resp.StatusCode)
		os.Exit(1)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error reading response body: %v\n", err)
		os.Exit(1)
	}

	matches := hrefPattern.FindAllSubmatch(body, -1)
	if len(matches) == 0 {
		fmt.Fprintf(os.Stderr, "no municipal court PDF links found on page\n")
		os.Exit(1)
	}

	var downloaded, skipped int
	for _, m := range matches {
		href := string(m[1])
		yymm := string(m[2])
		year := "20" + yymm[:2]
		month := yymm[2:]

		outName := fmt.Sprintf("municipal-courts-%s-%s.pdf", year, month)
		outPath := filepath.Join(*dir, outName)

		if _, err := os.Stat(outPath); err == nil {
			fmt.Fprintf(os.Stderr, "skip %s (already exists)\n", outName)
			skipped++
			continue
		}

		fullURL := "https://www.njcourts.gov" + href
		fmt.Fprintf(os.Stderr, "downloading %s -> %s\n", fullURL, outName)

		if err := downloadFile(fullURL, outPath); err != nil {
			fmt.Fprintf(os.Stderr, "error downloading %s: %v\n", fullURL, err)
			continue
		}
		downloaded++
	}

	fmt.Fprintf(os.Stderr, "Done: %d downloaded, %d skipped\n", downloaded, skipped)
}

func downloadFile(url, dest string) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("status %d", resp.StatusCode)
	}

	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = io.Copy(f, resp.Body)
	return err
}
