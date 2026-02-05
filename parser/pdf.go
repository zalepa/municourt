package parser

import (
	"bytes"
	"fmt"
	"os"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// PageData holds the extracted content stream and font CMap data for a single page.
type PageData struct {
	Content   []byte
	FontCMaps map[string]CMap // font name (e.g. "TT1") → CMap
}

// ContainsFilings checks whether the extracted text items contain "Filings",
// indicating a data page rather than a cover page.
func ContainsFilings(items []string) bool {
	for _, item := range items {
		if item == "Filings" {
			return true
		}
	}
	return false
}

// ExtractContentStreams opens a PDF file and returns the decompressed content
// stream bytes and font CMap data for each page.
func ExtractContentStreams(path string) ([]PageData, error) {
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

	var result []PageData
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

		fontCMaps := extractFontCMaps(ctx, pageDict)

		result = append(result, PageData{
			Content:   streamData,
			FontCMaps: fontCMaps,
		})
	}

	return result, nil
}

// extractFontCMaps extracts ToUnicode CMaps from each font in the page's
// resource dictionary.
func extractFontCMaps(ctx *model.Context, pageDict types.Dict) map[string]CMap {
	cmaps := make(map[string]CMap)

	resourcesObj, found := pageDict.Find("Resources")
	if !found {
		return cmaps
	}
	resourcesObj, err := ctx.Dereference(resourcesObj)
	if err != nil {
		return cmaps
	}
	resources, ok := resourcesObj.(types.Dict)
	if !ok {
		return cmaps
	}

	fontObj, found := resources.Find("Font")
	if !found {
		return cmaps
	}
	fontObj, err = ctx.Dereference(fontObj)
	if err != nil {
		return cmaps
	}
	fontDict, ok := fontObj.(types.Dict)
	if !ok {
		return cmaps
	}

	for fontName, fontRef := range fontDict {
		fontEntry, err := ctx.Dereference(fontRef)
		if err != nil {
			continue
		}
		fontEntryDict, ok := fontEntry.(types.Dict)
		if !ok {
			continue
		}

		tounicodeObj, found := fontEntryDict.Find("ToUnicode")
		if !found {
			continue
		}
		tounicodeObj, err = ctx.Dereference(tounicodeObj)
		if err != nil {
			continue
		}
		sd, ok := tounicodeObj.(types.StreamDict)
		if !ok {
			continue
		}
		if err := sd.Decode(); err != nil {
			continue
		}

		cmap := ParseCMap(sd.Content)
		if len(cmap) > 0 {
			cmaps[fontName] = cmap
		}
	}

	return cmaps
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
