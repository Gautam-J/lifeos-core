package main

import (
	"net/http"
	"strings"
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Gautam-J/lifeos-core/internal/testaccess"
)

// mcpTransport returns the transport of an MCP client that sends a valid
// token of Cloudflare Access with each request, as a client behind Access
// does.
func mcpTransport(addr string, access *testaccess.Access) *sdk.StreamableClientTransport {
	return &sdk.StreamableClientTransport{
		Endpoint:   "http://" + addr + "/mcp",
		HTTPClient: &http.Client{Transport: withToken(access.Token())},
	}
}

// withToken adds the token to each request.
type withToken string

func (token withToken) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set(testaccess.Header, string(token))
	return http.DefaultTransport.RoundTrip(r)
}

// initialize is the first request of an MCP client.
const initialize = `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"test","version":"0"}}}`

// postMCP sends the first request of an MCP client to the MCP listener.
// It sends the token when token is not empty, and it names host as the
// host of the request.
func postMCP(t *testing.T, addr, host, token string) (int, string) {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, "http://"+addr+"/mcp", strings.NewReader(initialize))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	if host != "" {
		req.Host = host
	}
	if token != "" {
		req.Header.Set(testaccess.Header, token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /mcp: %v", err)
	}
	defer resp.Body.Close()
	body := make([]byte, 4096)
	n, _ := resp.Body.Read(body)
	return resp.StatusCode, string(body[:n])
}

// LifeOS has no login of its own. A request that has no valid token did
// not come through Cloudflare Access, and the MCP server gives it nothing:
// no tool list and no tool call.
func TestServeRefusesAnMCPRequestThatHasNoAccessToken(t *testing.T) {
	cfg, _, access := startServer(t)

	claims := access.Claims()
	claims["aud"] = []string{strings.Repeat("f", 64)}
	for name, token := range map[string]string{
		"no token":                           "",
		"a token of a different application": access.Sign(claims),
	} {
		status, body := postMCP(t, cfg.MCPAddr, "", token)
		if status != http.StatusForbidden {
			t.Errorf("%s: status %d, want 403", name, status)
		}
		if strings.Contains(body, "lifeos") || strings.Contains(body, "jsonrpc") {
			t.Errorf("%s: the answer is an answer of the MCP server: %s", name, body)
		}
	}

	// The REST API and the admin pages are on a different listener, and
	// each has its own Access application. They need no token here.
	if status, _ := send(t, http.MethodGet, "http://"+cfg.HTTPAddr+"/api/items", "", ""); status != http.StatusOK {
		t.Errorf("GET /api/items: status %d, want 200", status)
	}
}

// The tunnel sends a request with the public hostname to the loopback
// listener. The MCP library refuses such a request by default, as a guard
// against a web page that reaches a local server. The token check is that
// guard here, so the server must answer.
func TestServeAnswersAnMCPRequestForThePublicHostname(t *testing.T) {
	cfg, _, access := startServer(t)

	status, body := postMCP(t, cfg.MCPAddr, "mcp.example.com", access.Token())

	if status != http.StatusOK || !strings.Contains(body, `"name":"lifeos"`) {
		t.Errorf("status %d, body %q; want 200 and the answer of the lifeos MCP server", status, body)
	}

	// The token check comes first for this hostname too.
	if status, _ := postMCP(t, cfg.MCPAddr, "mcp.example.com", ""); status != http.StatusForbidden {
		t.Errorf("no token: status %d, want 403", status)
	}
}

// A browser that has the login of Cloudflare Access sends that login with
// each request to the hostname, also with one that a page of a different
// site starts. Access then adds a valid token. The MCP server must refuse
// such a request, as the other listener does. An MCP client is no browser:
// it sends no header about its origin, and it goes through.
func TestServeRefusesAnMCPRequestFromAPageOfADifferentSite(t *testing.T) {
	cfg, _, access := startServer(t)

	for name, header := range map[string][2]string{
		"a request that the browser marks as cross-site": {"Sec-Fetch-Site", "cross-site"},
		"a request with the origin of a different site":  {"Origin", "https://evil.example"},
	} {
		req, err := http.NewRequest(http.MethodPost, "http://"+cfg.MCPAddr+"/mcp", strings.NewReader(initialize))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json, text/event-stream")
		req.Header.Set(testaccess.Header, access.Token())
		req.Header.Set(header[0], header[1])
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("%s: status %d, want 403", name, resp.StatusCode)
		}
	}

	// The same request with no such header is the request of an MCP client.
	if status, _ := postMCP(t, cfg.MCPAddr, "", access.Token()); status != http.StatusOK {
		t.Errorf("an MCP client: status %d, want 200", status)
	}
}

func TestAccessCheckComesFromTheEnvironment(t *testing.T) {
	aud := strings.Repeat("0123456789abcdef", 4)
	cases := []struct {
		name, team, aud, want string
	}{
		{"the two values", "my-team-01", aud, ""},
		{"no team", "", aud, "CF_ACCESS_TEAM"},
		{"a team that is a URL", "https://my-team.cloudflareaccess.com", aud, "CF_ACCESS_TEAM"},
		{"a team with a host of its own", "evil.example/x", aud, "CF_ACCESS_TEAM"},
		{"no audience tag", "my-team", "", "LIFEOS_MCP_AUD"},
		{"an audience tag that is too short", "my-team", "0123abcd", "LIFEOS_MCP_AUD"},
		{"an audience tag in capital letters", "my-team", strings.ToUpper(aud), "LIFEOS_MCP_AUD"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("CF_ACCESS_TEAM", tc.team)
			t.Setenv("LIFEOS_MCP_AUD", tc.aud)

			check, err := accessCheck()

			if tc.want == "" {
				if err != nil || check == nil {
					t.Fatalf("accessCheck: check %v, error %v; want a check", check, err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %v, want an error that names %s", err, tc.want)
			}
		})
	}
}
