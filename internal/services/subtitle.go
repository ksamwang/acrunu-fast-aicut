package services

import (
	"strings"
	"unicode"
)

func SubtitleDisplayText(text string) string {
	text = strings.Map(func(value rune) rune {
		if unicode.IsPunct(value) {
			return -1
		}
		return value
	}, text)
	return strings.Join(strings.Fields(text), " ")
}
