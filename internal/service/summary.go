package service

import (
	"strconv"
	"strings"
	"unicode/utf8"
)

// summary is the one line that search and the list of deleted records show for an item.
func (i Item) summary() string {
	s := i.Kind + ": " + i.Title
	if i.Rating != nil {
		s += ", rating " + strconv.FormatFloat(*i.Rating, 'f', -1, 64)
	}
	if i.OccurredOn != "" {
		s += ", on " + i.OccurredOn
	}
	return s
}

// shorten returns text as one line of at most max characters.
func shorten(text string, max int) string {
	line := strings.Join(strings.Fields(text), " ")
	if utf8.RuneCountInString(line) <= max {
		return line
	}
	return strings.TrimSpace(string([]rune(line)[:max])) + "…"
}
