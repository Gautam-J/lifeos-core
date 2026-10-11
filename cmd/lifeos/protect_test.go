package main

import (
	"net/http"
	"strings"
	"testing"
)

// post makes one POST with the headers that a browser adds, and returns
// the status.
func post(t *testing.T, url, contentType, body string, headers map[string]string) int {
	t.Helper()
	req, err := http.NewRequest("POST", url, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", contentType)
	for name, value := range headers {
		req.Header.Set(name, value)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST %s: %v", url, err)
	}
	resp.Body.Close()
	return resp.StatusCode
}

// A page of a different site can make the browser of Gautam send a POST,
// and the browser adds the login of Cloudflare Access to it. The server
// must refuse that request. A browser says where a request comes from in
// Sec-Fetch-Site, and an old browser in Origin.
func TestServeRefusesAPostFromAnotherSite(t *testing.T) {
	cfg, pool := startServer(t)
	rest := "http://" + cfg.HTTPAddr
	const form = "application/x-www-form-urlencoded"
	const item = `{"kind":"book","title":"Dune"}`

	refused := []struct {
		name, url, contentType, body string
		headers                      map[string]string
	}{
		{"an admin form from another site", rest + "/admin/items", form, "kind=book&title=Dune", map[string]string{"Sec-Fetch-Site": "cross-site"}},
		{"an admin form from an old browser", rest + "/admin/items", form, "kind=book&title=Dune", map[string]string{"Origin": "https://evil.example"}},
		{"an admin form from a frame that has no origin", rest + "/admin/items", form, "kind=book&title=Dune", map[string]string{"Origin": "null"}},
		{"an API call from another site", rest + "/api/items", "application/json", item, map[string]string{"Sec-Fetch-Site": "cross-site"}},
		{"an API call from a page of the same domain", rest + "/api/items", "application/json", item, map[string]string{"Sec-Fetch-Site": "same-site"}},
	}
	for _, r := range refused {
		if status := post(t, r.url, r.contentType, r.body, r.headers); status != http.StatusForbidden {
			t.Errorf("%s: status = %d, want 403", r.name, status)
		}
	}
	var stored int
	if err := pool.QueryRow(t.Context(), "SELECT count(*) FROM catalog_items").Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored != 0 {
		t.Fatalf("a refused request stored %d items", stored)
	}

	accepted := []struct {
		name, url, contentType, body string
		headers                      map[string]string
	}{
		{"an admin form from its own page", rest + "/admin/items", form, "kind=book&title=Dune", map[string]string{"Sec-Fetch-Site": "same-origin"}},
		{"an admin form from an old browser on its own page", rest + "/admin/items", form, "kind=book&title=Emma", map[string]string{"Origin": rest}},
		{"an API call from a program, which sends no such header", rest + "/api/items", "application/json", item, nil},
	}
	for _, a := range accepted {
		if status := post(t, a.url, a.contentType, a.body, a.headers); status >= 300 {
			t.Errorf("%s: status = %d, want success", a.name, status)
		}
	}
	if err := pool.QueryRow(t.Context(), "SELECT count(*) FROM catalog_items").Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored != 3 {
		t.Errorf("the accepted requests stored %d items, want 3", stored)
	}

	// A GET changes nothing, so a link from another site can open a page.
	req, err := http.NewRequest("GET", rest+"/admin/items", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Sec-Fetch-Site", "cross-site")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("a GET from another site: status = %d, want 200", resp.StatusCode)
	}
}
