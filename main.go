package main

import (
	"fmt"
	"os"

	"github.com/zalepa/municourt/cmd"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "parse":
		cmd.Parse(os.Args[2:])
	case "download":
		cmd.Download(os.Args[2:])
	default:
		usage()
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintf(os.Stderr, "Usage: municourt <command>\n\nCommands:\n  parse      Parse municipal court PDF statistics\n  download   Download municipal court PDFs from njcourts.gov\n")
}
