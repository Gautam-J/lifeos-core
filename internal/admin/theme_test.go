package admin_test

import (
	"net/http"
	"regexp"
	"strings"
	"testing"
)

// The theme is a choice of the browser: the system setting, or the one
// that Gautam set there. The server stores nothing and gets nothing.
func TestPagesHaveTheThemeChoice(t *testing.T) {
	h, _ := newTestAdmin(t)

	page := get(h, "/admin/items").Body.String()

	for _, want := range []string{
		`data-theme-set="system"`,
		`data-theme-set="light"`,
		`data-theme-set="dark"`,
	} {
		if !strings.Contains(page, want) {
			t.Errorf("the page does not contain %s", want)
		}
	}
	// The script sets the theme before the first paint, so it must not
	// wait for the page and it must come before the stylesheet.
	script := regexp.MustCompile(`<script src="(/admin/static/theme\.js\?v=[0-9a-f]{12})"></script>`).FindStringSubmatchIndex(page)
	if script == nil {
		t.Fatal("the page does not load the theme script, with a version and with no defer")
	}
	if sheet := strings.Index(page, `rel="stylesheet"`); sheet < script[0] {
		t.Error("the theme script comes after the stylesheet")
	}
	rec := get(h, page[script[2]:script[3]])
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "localStorage") {
		t.Errorf("theme script: status = %d, and it does not use localStorage", rec.Code)
	}
}

// The dark colors are in the stylesheet two times: for the system setting
// and for the explicit choice. CSS has no way to say them one time for
// each browser that Gautam uses. The two blocks must stay equal.
func TestTheTwoBlocksOfDarkColorsAreEqual(t *testing.T) {
	h, _ := newTestAdmin(t)
	css := get(h, "/admin/static/app.css").Body.String()

	block := func(selector string) string {
		t.Helper()
		start := strings.Index(css, selector+" {")
		if start < 0 {
			t.Fatalf("the stylesheet has no rule for %s", selector)
		}
		body := css[start+len(selector)+2:]
		lines := strings.Split(body[:strings.Index(body, "}")], "\n")
		for i := range lines {
			lines[i] = strings.TrimSpace(lines[i])
		}
		return strings.Join(lines, "\n")
	}
	system := block(`:root:not([data-theme="light"])`)
	explicit := block(`:root[data-theme="dark"]`)
	if !strings.Contains(explicit, "--canvas: #181715;") {
		t.Errorf("the dark block does not set the canvas:\n%s", explicit)
	}
	if system != explicit {
		t.Errorf("the dark colors for the system setting are not the dark colors of the choice:\n%s\n---\n%s", system, explicit)
	}
	if !strings.Contains(css, "@media (prefers-color-scheme: dark)") {
		t.Error("the stylesheet does not follow the system setting")
	}
}
