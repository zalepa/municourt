package parser

import (
	"math"
	"strconv"
	"strings"
)

// kerningThreshold is the absolute value above which a kerning/spacing number
// in a TJ array is treated as a column separator rather than intra-word spacing.
const kerningThreshold = 500

// ExtractTextItems parses a PDF content stream and returns an ordered list of
// text strings. Empty strings ("") are inserted as line-break markers whenever
// a TD/Td operator moves to a new line (non-zero y offset).
func ExtractTextItems(stream []byte) []string {
	tokens := tokenize(string(stream))
	var items []string
	var stack []token   // operand stack
	var tc float64      // current Tc (character spacing) in text space units

	for i := 0; i < len(tokens); i++ {
		t := tokens[i]
		switch t.kind {
		case tokOperator:
			switch t.value {
			case "Tj":
				// Single string show: the operand is the string on the stack.
				if len(stack) > 0 {
					s := stack[len(stack)-1]
					if s.kind == tokString {
						tcThousandths := tc * 1000
						if math.Abs(tcThousandths) > kerningThreshold {
							// Large Tc: each character is visually in a
							// different column, so emit them separately.
							for _, ch := range s.value {
								items = append(items, string(ch))
							}
						} else {
							items = append(items, s.value)
						}
					}
				}
				stack = stack[:0]

			case "TJ":
				// Array show: the operand is the array on the stack.
				if len(stack) > 0 {
					a := stack[len(stack)-1]
					if a.kind == tokArray {
						items = append(items, processTJArray(a.children, tc*1000)...)
					}
				}
				stack = stack[:0]

			case "TD", "Td":
				// Text positioning. Two numeric operands: tx ty.
				// A non-zero ty means we moved to a new line.
				if len(stack) >= 2 {
					tyStr := stack[len(stack)-1].value
					ty, err := strconv.ParseFloat(tyStr, 64)
					if err == nil && ty != 0 {
						items = append(items, "")
					}
				}
				stack = stack[:0]

			case "Tm":
				// Text matrix — always starts a new positioned block.
				items = append(items, "")
				stack = stack[:0]

			case "Tc":
				// Character spacing operator: one numeric operand.
				if len(stack) > 0 {
					val, err := strconv.ParseFloat(stack[len(stack)-1].value, 64)
					if err == nil {
						tc = val
					}
				}
				stack = stack[:0]

			default:
				// Other operators: clear the operand stack.
				stack = stack[:0]
			}

		default:
			stack = append(stack, t)
		}
	}

	return items
}

// processTJArray takes the children of a TJ array and returns text items,
// using the effective gap between characters to decide column boundaries.
//
// The effective gap accounts for both TJ displacement values and Tc (character
// spacing). When Tc is large, the PDF spreads characters across columns using
// character spacing instead of (or in addition to) TJ kerning values. TJ values
// that approximately equal Tc*1000 cancel the character spacing, keeping
// adjacent characters together within the same value.
//
// For each pair of adjacent characters:
//   - Within a string: gap = Tc*1000 (no TJ value)
//   - Across a TJ number: gap = Tc*1000 - TJ_value
//
// If abs(gap) > kerningThreshold, a column boundary is inserted.
func processTJArray(children []token, tcThousandths float64) []string {
	var items []string
	var cur strings.Builder
	nextGap := 0.0
	isFirst := true

	for _, c := range children {
		switch c.kind {
		case tokString:
			for _, ch := range c.value {
				if !isFirst && cur.Len() > 0 && math.Abs(nextGap) > kerningThreshold {
					items = append(items, cur.String())
					cur.Reset()
				}
				cur.WriteRune(ch)
				isFirst = false
				nextGap = tcThousandths // default for next char (intra-string)
			}
		case tokNumber:
			val, err := strconv.ParseFloat(c.value, 64)
			if err != nil {
				continue
			}
			// Subtract TJ value from the pending gap. The TJ displacement
			// is subtracted from the text position, so it reduces the
			// effective gap when positive and increases it when negative.
			nextGap -= val
		}
	}

	if cur.Len() > 0 {
		items = append(items, cur.String())
	}

	return items
}

// Token types for the PDF content stream tokenizer.
type tokenKind int

const (
	tokString   tokenKind = iota // (text)
	tokNumber                    // 123, -45.6
	tokOperator                  // BT, Tj, TJ, TD, etc.
	tokArray                     // [...] — children stored in token.children
)

type token struct {
	kind     tokenKind
	value    string
	children []token // only for tokArray
}

// tokenize performs a simple tokenization of a PDF content stream.
func tokenize(s string) []token {
	var tokens []token
	i := 0
	n := len(s)

	for i < n {
		ch := s[i]

		// Skip whitespace.
		if ch == ' ' || ch == '\t' || ch == '\r' || ch == '\n' {
			i++
			continue
		}

		// Comment.
		if ch == '%' {
			for i < n && s[i] != '\n' && s[i] != '\r' {
				i++
			}
			continue
		}

		// String literal (parenthesized).
		if ch == '(' {
			str, end := readString(s, i)
			tokens = append(tokens, token{kind: tokString, value: str})
			i = end
			continue
		}

		// Array.
		if ch == '[' {
			arr, end := readArray(s, i)
			tokens = append(tokens, arr)
			i = end
			continue
		}

		// Number (including negative and decimal).
		if ch == '-' || ch == '+' || ch == '.' || (ch >= '0' && ch <= '9') {
			start := i
			if ch == '-' || ch == '+' {
				i++
			}
			for i < n && ((s[i] >= '0' && s[i] <= '9') || s[i] == '.') {
				i++
			}
			tokens = append(tokens, token{kind: tokNumber, value: s[start:i]})
			continue
		}

		// Operator or name.
		if ch == '/' {
			// Name object — skip it (we don't need font names etc. as tokens).
			i++
			for i < n && s[i] != ' ' && s[i] != '\t' && s[i] != '\r' && s[i] != '\n' &&
				s[i] != '/' && s[i] != '(' && s[i] != '[' && s[i] != '<' {
				i++
			}
			continue
		}

		// Hex string <...>
		if ch == '<' {
			// Skip hex strings and dict markers.
			i++
			depth := 1
			for i < n && depth > 0 {
				if s[i] == '<' {
					depth++
				} else if s[i] == '>' {
					depth--
				}
				i++
			}
			continue
		}

		// Skip '>' that isn't part of a hex string.
		if ch == ']' || ch == '>' {
			i++
			continue
		}

		// Keyword / operator.
		start := i
		for i < n && s[i] != ' ' && s[i] != '\t' && s[i] != '\r' && s[i] != '\n' &&
			s[i] != '(' && s[i] != '[' && s[i] != '/' && s[i] != '<' {
			i++
		}
		word := s[start:i]
		if word != "" {
			tokens = append(tokens, token{kind: tokOperator, value: word})
		}
	}

	return tokens
}

// readString reads a parenthesized string starting at s[pos]=='(' and returns
// the string content and the index after the closing ')'.
func readString(s string, pos int) (string, int) {
	var buf strings.Builder
	i := pos + 1 // skip opening '('
	depth := 1
	n := len(s)

	for i < n && depth > 0 {
		ch := s[i]
		if ch == '\\' && i+1 < n {
			i++
			next := s[i]
			switch next {
			case 'n':
				buf.WriteByte('\n')
			case 'r':
				buf.WriteByte('\r')
			case 't':
				buf.WriteByte('\t')
			case '(', ')', '\\':
				buf.WriteByte(next)
			default:
				// Octal escape or unknown — just emit.
				if next >= '0' && next <= '7' {
					oct := string(next)
					for j := 0; j < 2 && i+1 < n && s[i+1] >= '0' && s[i+1] <= '7'; j++ {
						i++
						oct += string(s[i])
					}
					val, _ := strconv.ParseInt(oct, 8, 32)
					buf.WriteByte(byte(val))
				} else {
					buf.WriteByte(next)
				}
			}
		} else if ch == '(' {
			depth++
			buf.WriteByte(ch)
		} else if ch == ')' {
			depth--
			if depth > 0 {
				buf.WriteByte(ch)
			}
		} else {
			buf.WriteByte(ch)
		}
		i++
	}

	return buf.String(), i
}

// readArray reads a [...] array starting at s[pos]=='[' and returns a tokArray
// token with children, plus the index after the closing ']'.
func readArray(s string, pos int) (token, int) {
	var children []token
	i := pos + 1 // skip '['
	n := len(s)

	for i < n {
		ch := s[i]

		if ch == ' ' || ch == '\t' || ch == '\r' || ch == '\n' {
			i++
			continue
		}

		if ch == ']' {
			i++
			break
		}

		if ch == '(' {
			str, end := readString(s, i)
			children = append(children, token{kind: tokString, value: str})
			i = end
			continue
		}

		// Number.
		if ch == '-' || ch == '+' || ch == '.' || (ch >= '0' && ch <= '9') {
			start := i
			if ch == '-' || ch == '+' {
				i++
			}
			for i < n && ((s[i] >= '0' && s[i] <= '9') || s[i] == '.') {
				i++
			}
			children = append(children, token{kind: tokNumber, value: s[start:i]})
			continue
		}

		// Skip anything else inside the array.
		i++
	}

	return token{kind: tokArray, children: children}, i
}
