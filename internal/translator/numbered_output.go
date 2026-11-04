package translator

import (
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

// Keep coverage consistent with frontend/src/utils/translation.ts: accepted
// display numbering must not cause a second request, and hidden markers cannot
// make an empty region look complete.
var numberedOutputLine = regexp.MustCompile(`^(?:\[(\d+)\]|(\d+)[.)、:：-])\s*(.*)$`)
var numberedOutputMarker = regexp.MustCompile(`(?i)\bOCR_TEXT_(?:BEGIN|END)\b[:：]?\s*`)
var numberedOutputSeparator = regexp.MustCompile("(?i)^(?:[-_*`:\\s]*OCR[-_\\s]*TEXT[-_\\s]*(?:BEGIN|END)[-_*`:\\s]*|[-_*`:]{2,}\\s*(?:BEGIN|END)[-_*`:\\s]*|(?:BEGIN|END)\\s*[-_*`:]{2,})$")

func cleanNumberedOutputLine(line string) string {
	trim := func(text string) string {
		return strings.TrimFunc(text, func(r rune) bool { return unicode.IsSpace(r) || r == '\ufeff' })
	}
	line = trim(line)
	line = trim(strings.Trim(line, "\"'“”‘’"))
	line = trim(numberedOutputMarker.ReplaceAllString(line, ""))
	if line == "```" || numberedOutputSeparator.MatchString(line) {
		return ""
	}
	return trim(strings.Trim(line, "\"'“”‘’"))
}

func parseNumberedTranslationIndices(translatedText string) map[int]bool {
	indices := make(map[int]bool)
	normalized := strings.ReplaceAll(strings.ReplaceAll(translatedText, "\r\n", "\n"), "\r", "\n")
	for _, line := range strings.Split(normalized, "\n") {
		match := numberedOutputLine.FindStringSubmatch(cleanNumberedOutputLine(line))
		if match == nil || cleanNumberedOutputLine(match[3]) == "" {
			continue
		}
		id := match[1]
		if id == "" {
			id = match[2]
		}
		if index, err := strconv.Atoi(id); err == nil && index > 0 {
			indices[index] = true
		}
	}
	return indices
}
