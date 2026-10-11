package service

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

// checkText refuses text that Postgres cannot store, and text that holds a
// secret (AGENTS.md rule 7). The message names the kind of secret and does
// not repeat it. Each text that a caller gives goes through this check
// before the service stores it.
func checkText(field, value string) error {
	if !utf8.ValidString(value) || strings.ContainsRune(value, 0) {
		return invalid(field, "must be valid text without a NUL character")
	}
	if kind := secretKind(value); kind != "" {
		return invalid(field, "must not hold "+kind+": LifeOS does not store one. Take it out, then send the text again")
	}
	return nil
}

// checkLimit returns the number of rows for a list. 0 means 50; the
// maximum is 200.
func checkLimit(limit int) (int32, error) {
	if limit == 0 {
		limit = defaultListLimit
	}
	if limit < 1 || limit > maxListLimit {
		return 0, invalid("limit", fmt.Sprintf("must be from 1 to %d", maxListLimit))
	}
	return int32(limit), nil
}

// today returns the IST date of now. The time is 00:00 UTC, as a date
// column reads.
func today(now time.Time) time.Time {
	y, m, d := now.In(IST).Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// parseDay reads a day as YYYY-MM-DD.
func parseDay(field, value string) (time.Time, error) {
	day, err := time.Parse(time.DateOnly, value)
	if err != nil {
		return time.Time{}, invalid(field, "must be a date as YYYY-MM-DD")
	}
	return day, nil
}

// pastDay reads a day as YYYY-MM-DD that is not later than today in IST.
// An empty value means today.
func pastDay(field, value string, now time.Time) (time.Time, error) {
	if value == "" {
		return today(now), nil
	}
	day, err := parseDay(field, value)
	if err != nil {
		return day, err
	}
	if day.After(today(now)) {
		return day, invalid(field, "must not be in the future")
	}
	return day, nil
}
