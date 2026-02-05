package parser

import (
	"encoding/binary"
	"encoding/hex"
	"strings"
)

// CMap maps 2-byte glyph IDs to Unicode runes, parsed from a ToUnicode CMap stream.
type CMap map[uint16]rune

// ParseCMap extracts glyph-to-unicode mappings from a ToUnicode CMap stream.
// It handles beginbfchar/endbfchar (single mappings) and beginbfrange/endbfrange
// (range mappings).
func ParseCMap(data []byte) CMap {
	cmap := make(CMap)
	s := string(data)

	// Parse all bfchar sections.
	for {
		start := strings.Index(s, "beginbfchar")
		if start < 0 {
			break
		}
		s = s[start+len("beginbfchar"):]
		end := strings.Index(s, "endbfchar")
		if end < 0 {
			break
		}
		section := s[:end]
		s = s[end+len("endbfchar"):]
		parseBFChar(section, cmap)
	}

	// Reset and parse all bfrange sections.
	s = string(data)
	for {
		start := strings.Index(s, "beginbfrange")
		if start < 0 {
			break
		}
		s = s[start+len("beginbfrange"):]
		end := strings.Index(s, "endbfrange")
		if end < 0 {
			break
		}
		section := s[:end]
		s = s[end+len("endbfrange"):]
		parseBFRange(section, cmap)
	}

	return cmap
}

// parseBFChar parses lines like: <0003> <0020>
func parseBFChar(section string, cmap CMap) {
	tokens := extractHexTokens(section)
	for i := 0; i+1 < len(tokens); i += 2 {
		src := decodeUint16(tokens[i])
		dst := decodeUint16(tokens[i+1])
		cmap[src] = rune(dst)
	}
}

// parseBFRange parses lines like: <0024> <003d> <0041>
func parseBFRange(section string, cmap CMap) {
	tokens := extractHexTokens(section)
	for i := 0; i+2 < len(tokens); i += 3 {
		lo := decodeUint16(tokens[i])
		hi := decodeUint16(tokens[i+1])
		dstStart := decodeUint16(tokens[i+2])
		for g := lo; g <= hi; g++ {
			cmap[g] = rune(dstStart + (g - lo))
		}
	}
}

// extractHexTokens pulls all <hex> tokens from a string.
func extractHexTokens(s string) []string {
	var tokens []string
	for {
		start := strings.IndexByte(s, '<')
		if start < 0 {
			break
		}
		end := strings.IndexByte(s[start+1:], '>')
		if end < 0 {
			break
		}
		end += start + 1
		tokens = append(tokens, s[start+1:end])
		s = s[end+1:]
	}
	return tokens
}

// decodeUint16 decodes a hex string (e.g. "0041") to a uint16.
func decodeUint16(h string) uint16 {
	b, err := hex.DecodeString(strings.TrimSpace(h))
	if err != nil || len(b) < 2 {
		return 0
	}
	return binary.BigEndian.Uint16(b[:2])
}

// DecodeHexString decodes a hex-encoded glyph string using a CMap.
// The hex string contains 2-byte big-endian glyph IDs (e.g. "003000380031").
// Each pair of bytes is looked up in the CMap to produce a Unicode rune.
func DecodeHexString(hexStr string, cmap CMap) string {
	// Remove any whitespace in the hex string.
	hexStr = strings.ReplaceAll(hexStr, " ", "")
	hexStr = strings.ReplaceAll(hexStr, "\n", "")
	hexStr = strings.ReplaceAll(hexStr, "\r", "")

	b, err := hex.DecodeString(hexStr)
	if err != nil {
		return ""
	}

	var buf strings.Builder
	for i := 0; i+1 < len(b); i += 2 {
		gid := binary.BigEndian.Uint16(b[i : i+2])
		if r, ok := cmap[gid]; ok {
			buf.WriteRune(r)
		}
	}
	return buf.String()
}
