package terminal

import (
	"regexp"
	"strings"
	"unicode"
)

var ansiSequencePattern = regexp.MustCompile(`\x1b\[[0-9;]*[A-Za-z]`)
var controlCodePattern = regexp.MustCompile(`[\x00-\x08\x0B\x0C\x0E-\x1F]`)

// StripANSI removes ANSI CSI escape sequences and control codes while preserving
// tabs, newlines, and carriage returns.
func StripANSI(input string) string {
	if input == "" {
		return input
	}
	cleaned := ansiSequencePattern.ReplaceAllString(input, "")
	if cleaned == "" {
		return cleaned
	}
	return controlCodePattern.ReplaceAllString(cleaned, "")
}

// StripRepeatedChars collapses repeated non-whitespace characters to a single
// instance when the run length is at least minLen.
func StripRepeatedChars(input string, minLen int) string {
	if input == "" || minLen <= 1 {
		return input
	}
	var builder strings.Builder
	builder.Grow(len(input))

	var last rune
	count := 0
	appendRun := func(r rune, runLength int) {
		if runLength == 0 {
			return
		}
		if unicode.IsSpace(r) || runLength < minLen {
			for i := 0; i < runLength; i++ {
				builder.WriteRune(r)
			}
			return
		}
		builder.WriteRune(r)
	}

	for _, r := range input {
		if count == 0 {
			last = r
			count = 1
			continue
		}
		if r == last {
			count++
			continue
		}
		appendRun(last, count)
		last = r
		count = 1
	}
	appendRun(last, count)
	return builder.String()
}

// FilterTerminalOutput strips ANSI/control codes and collapses repeated
// character runs for log-friendly terminal output.
func FilterTerminalOutput(input string) string {
	if input == "" {
		return input
	}
	cleaned := StripANSI(input)
	return StripRepeatedChars(cleaned, 3)
}
