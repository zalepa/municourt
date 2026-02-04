package parser

import (
	"bytes"
	"fmt"
	"os"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// ExtractContentStreams opens a PDF file and returns the decompressed content
// stream bytes for each page. Pages whose content stream does not contain
// "Filings" (e.g. cover pages) are skipped.
func ExtractContentStreams(path string) ([][]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open pdf: %w", err)
	}
	defer f.Close()

	ctx, err := pdfcpu.Read(f, model.NewDefaultConfiguration())
	if err != nil {
		return nil, fmt.Errorf("read pdf: %w", err)
	}

	if err := pdfcpu.OptimizeXRefTable(ctx); err != nil {
		return nil, fmt.Errorf("optimize xref: %w", err)
	}

	if err := ctx.EnsurePageCount(); err != nil {
		return nil, fmt.Errorf("page count: %w", err)
	}

	var result [][]byte
	for i := 1; i <= ctx.PageCount; i++ {
		pageDict, _, _, err := ctx.PageDict(i, false)
		if err != nil {
			return nil, fmt.Errorf("page %d dict: %w", i, err)
		}

		obj, found := pageDict.Find("Contents")
		if !found {
			continue
		}

		streamData, err := resolveContentStream(ctx, obj)
		if err != nil {
			return nil, fmt.Errorf("page %d content stream: %w", i, err)
		}

		// Skip pages that don't have table data (e.g. cover pages).
		if !bytes.Contains(streamData, []byte("Filings")) {
			continue
		}

		result = append(result, streamData)
	}

	return result, nil
}

// resolveContentStream dereferences and decompresses a Contents entry, which
// may be a single stream or an array of streams.
func resolveContentStream(ctx *model.Context, obj types.Object) ([]byte, error) {
	obj, err := ctx.Dereference(obj)
	if err != nil {
		return nil, err
	}

	switch v := obj.(type) {
	case types.StreamDict:
		if err := v.Decode(); err != nil {
			return nil, fmt.Errorf("decode stream: %w", err)
		}
		return v.Content, nil

	case types.Array:
		var buf bytes.Buffer
		for _, item := range v {
			data, err := resolveContentStream(ctx, item)
			if err != nil {
				return nil, err
			}
			buf.Write(data)
			buf.WriteByte('\n')
		}
		return buf.Bytes(), nil

	default:
		return nil, fmt.Errorf("unexpected Contents type: %T", obj)
	}
}
