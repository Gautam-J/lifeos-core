package admin

import (
	"strconv"
	"time"

	"github.com/Gautam-J/lifeos-core/internal/service"
)

// stamp shows a moment in IST, as each list does.
func stamp(t time.Time) string {
	return t.In(service.IST).Format("2 Jan 2006 15:04")
}

// clock shows the time of day in IST.
func clock(t time.Time) string {
	return t.In(service.IST).Format("15:04")
}

// longDay shows a day that the service gave as YYYY-MM-DD in words:
// "Monday, 12 October". A text that is not a day stays as it is.
func longDay(day string) string {
	t, err := time.Parse(time.DateOnly, day)
	if err != nil {
		return day
	}
	return t.Format("Monday, 2 January")
}

// shortDay shows a day as "12 Oct 2026".
func shortDay(day string) string {
	t, err := time.Parse(time.DateOnly, day)
	if err != nil {
		return day
	}
	return t.Format("2 Jan 2006")
}

// count returns a number with its noun: "1 todo", "3 todos".
func count(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return strconv.Itoa(n) + " " + many
}
