package parser

import (
	"testing"
)

func TestExtractTextItems_TJKerning(t *testing.T) {
	// Simulates a TJ array with kerning-based concatenation.
	// (8)0(8) should concatenate to "88", and -4704.6 should separate.
	stream := []byte(`BT
[(8)0(8)-4704.6(2)0(3)]TJ
ET`)

	items := ExtractTextItems(stream)

	// Filter out empty line-break markers.
	var nonEmpty []string
	for _, s := range items {
		if s != "" {
			nonEmpty = append(nonEmpty, s)
		}
	}

	if len(nonEmpty) != 2 {
		t.Fatalf("expected 2 items, got %d: %v", len(nonEmpty), nonEmpty)
	}
	if nonEmpty[0] != "88" {
		t.Errorf("expected first item '88', got %q", nonEmpty[0])
	}
	if nonEmpty[1] != "23" {
		t.Errorf("expected second item '23', got %q", nonEmpty[1])
	}
}

func TestExtractTextItems_Tj(t *testing.T) {
	stream := []byte(`BT
(Hello World)Tj
ET`)

	items := ExtractTextItems(stream)

	var nonEmpty []string
	for _, s := range items {
		if s != "" {
			nonEmpty = append(nonEmpty, s)
		}
	}

	if len(nonEmpty) != 1 {
		t.Fatalf("expected 1 item, got %d: %v", len(nonEmpty), nonEmpty)
	}
	if nonEmpty[0] != "Hello World" {
		t.Errorf("expected 'Hello World', got %q", nonEmpty[0])
	}
}

func TestExtractTextItems_TDLineBreaks(t *testing.T) {
	stream := []byte(`BT
(Line1)Tj
0 -12 TD
(Line2)Tj
ET`)

	items := ExtractTextItems(stream)

	// Should have: "", "Line1", "", "Line2" (with line-break markers).
	var foundLine1, foundLine2 bool
	var breakBetween bool
	for i, s := range items {
		if s == "Line1" {
			foundLine1 = true
		}
		if s == "Line2" {
			foundLine2 = true
		}
		// Check there's a break between Line1 and Line2.
		if foundLine1 && !foundLine2 && s == "" && i > 0 {
			breakBetween = true
		}
	}

	if !foundLine1 || !foundLine2 {
		t.Errorf("expected both Line1 and Line2, got items: %v", items)
	}
	if !breakBetween {
		t.Errorf("expected line break between Line1 and Line2, got items: %v", items)
	}
}

func TestExtractTextItems_SmallKerningConcatenates(t *testing.T) {
	// Small kerning values (abs <= 500) should concatenate strings.
	stream := []byte(`BT
[(H)-50(e)-30(l)(l)(o)]TJ
ET`)

	items := ExtractTextItems(stream)

	var nonEmpty []string
	for _, s := range items {
		if s != "" {
			nonEmpty = append(nonEmpty, s)
		}
	}

	if len(nonEmpty) != 1 {
		t.Fatalf("expected 1 item, got %d: %v", len(nonEmpty), nonEmpty)
	}
	if nonEmpty[0] != "Hello" {
		t.Errorf("expected 'Hello', got %q", nonEmpty[0])
	}
}

func TestExtractTextItems_MixedTjAndTJ(t *testing.T) {
	stream := []byte(`BT
(MUNICIPAL COURT STATISTICS)Tj
2.1882 -1.4941 TD
(JULY 2023 - JUNE 2024)Tj
3.0706 -1.4941 TD
(ATLANTIC)Tj
-.0118 -1.4941 TD
(ABSECON)Tj
0 8.52 -8.52 0 101.52 285.96 Tm
[(D.P. &)-3012.9(Other)-2811.9(Criminal)]TJ
ET`)

	items := ExtractTextItems(stream)

	var nonEmpty []string
	for _, s := range items {
		if s != "" {
			nonEmpty = append(nonEmpty, s)
		}
	}

	expected := []string{
		"MUNICIPAL COURT STATISTICS",
		"JULY 2023 - JUNE 2024",
		"ATLANTIC",
		"ABSECON",
		"D.P. &", "Other", "Criminal",
	}

	if len(nonEmpty) != len(expected) {
		t.Fatalf("expected %d items, got %d: %v", len(expected), len(nonEmpty), nonEmpty)
	}
	for i, exp := range expected {
		if nonEmpty[i] != exp {
			t.Errorf("item %d: expected %q, got %q", i, exp, nonEmpty[i])
		}
	}
}

func TestTokenizeEscapedParens(t *testing.T) {
	stream := []byte(`BT
(\(moving\))Tj
ET`)

	items := ExtractTextItems(stream)

	var nonEmpty []string
	for _, s := range items {
		if s != "" {
			nonEmpty = append(nonEmpty, s)
		}
	}

	if len(nonEmpty) != 1 {
		t.Fatalf("expected 1 item, got %d: %v", len(nonEmpty), nonEmpty)
	}
	if nonEmpty[0] != "(moving)" {
		t.Errorf("expected '(moving)', got %q", nonEmpty[0])
	}
}
