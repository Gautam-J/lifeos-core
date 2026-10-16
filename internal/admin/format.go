package admin

import (
	"strconv"
	"strings"
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

// dayLayout is a day as Gautam types it in a field of a day (#50).
const dayLayout = "02-01-2006"

// dayIn turns the text of a field of a day into YYYY-MM-DD for the service.
// A text that is not a day as DD-MM-YYYY stays as it is, so that the
// service refuses it, or reads it when it is YYYY-MM-DD already.
func dayIn(text string) string {
	t, err := time.Parse(dayLayout, strings.TrimSpace(text))
	if err != nil {
		return text
	}
	return t.Format(time.DateOnly)
}

// dayOut shows a day that the service gave as YYYY-MM-DD in a field of a
// day. A text that is not such a day stays as it is: it is what Gautam
// typed, and the form shows it again with the reason.
func dayOut(day string) string {
	t, err := time.Parse(time.DateOnly, day)
	if err != nil {
		return day
	}
	return t.Format(dayLayout)
}

// reason is the text of a refusal of the service for the admin pages. A day
// is DD-MM-YYYY there, and the service names the format of the API.
func reason(err *service.ValidationError) string {
	return strings.ReplaceAll(err.Error(), "YYYY-MM-DD", "DD-MM-YYYY")
}
