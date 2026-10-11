# Phase 3 MCP Hostname Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** The Claude app on the phone and Claude Code on the Pi use the `lifeos` MCP server through one public hostname, the server answers no request that did not come through the login of Cloudflare Access, and each commit of the wiki gets to GitHub.

**Architecture:** Cloudflare Access does the OAuth login for the MCP client (managed OAuth) and adds a signed token to each request that it lets through. The MCP handler verifies that token itself: the signature against the public keys of the team, the audience tag of the Access application and the time of expiry. The server holds no OAuth code and has no mode that needs no token. The server does not push the wiki; a job with the SSH key of the user fetches the commits into a mirror that the server cannot write, and pushes from there.

**Tech Stack:** No new dependency and no migration. Go 1.26.0 standard library (`crypto/rsa`, `encoding/json`), MCP Go SDK v1.8.0 (`StreamableHTTPOptions.DisableLocalhostProtection`), bash, git 2.39, systemd 252, cloudflared 2026.10, Cloudflare Access.

**Spec:** `docs/design.md` (sections "Access layer", "Agent layer" and "Deployment") and `AGENTS.md` (section "Deploy"). Read both before you start a task. The section "Decisions in this plan" lists where this plan differs from the design doc: those decisions take precedence. The plan before this one is `docs/superpowers/plans/2026-10-11-phase3-wiki-tools.md`.

## Changes after the review

Tasks 1 to 6 were executed on 2026-10-11. One reviewer read the full change and the commands of Task 7 before Task 7. It found no critical problem. Four findings were corrected, each with a test or a check that failed first. The code blocks of Tasks 1 to 6 show the code before these corrections; the repository has the code after them.

| Change | Where | Why |
| --- | --- | --- |
| The unit of the wiki push has `TimeoutStartSec=10min`. | `deploy/systemd/lifeos-wiki-push.service`, `deploy/test.sh` | A oneshot unit has no time limit. A push over a dead connection kept the unit in its start for good, and no later run started. |
| `wiki-push.sh` sets `GIT_NO_LAZY_FETCH=1`. | `deploy/wiki-push.sh`, `deploy/test.sh` | A fetch from a partial clone can get an absent object with a program that a setting of the source names. git has this off only by default. The new case of `deploy/test.sh` has a control. |
| `wiki-push.sh` pushes only commits that change files below `wiki/`. | `deploy/wiki-push.sh`, `deploy/test.sh`, `AGENTS.md` | The server can write `.claude/settings.json` in the live repository. Through GitHub, that file gets to `~/lifeos-wiki`, where a Claude Code session reads it. |
| The MCP listener has `http.CrossOriginProtection`, as the other listener has. | `cmd/lifeos/serve.go`, `cmd/lifeos/access_test.go`, `internal/mcp/server.go`, `AGENTS.md` | Access adds its token also to a request that a page of a different site starts in a browser that has the login. Only a rule of the MCP library stopped such a request. The connector of Claude sends no header about its origin. |
| The commands of Task 7 stop before a change when a check fails, give no API token on a command line, and do the first push of Step 3 by hand. | This file | The reviewer found blocks that continued after a failed check, also before the restart of `cloudflared`. The new guard of the push refuses the commit of Step 2, which is correct. |

New test: `TestServeRefusesAnMCPRequestFromAPageOfADifferentSite`. New cases of `deploy/test.sh`: the time limit of the unit, a live repository that is a partial clone, and a commit with a file that is not a wiki page.

Nine smaller findings were not corrected. They are in the report to Gautam.

## Decisions in this plan

Gautam approved decisions 1 to 4 on 2026-10-11, after a spike with a throwaway server. He approved decisions 5, 7, 10 and 13 in the review of this plan. The other decisions follow from those.

| # | Decision | Reason |
| --- | --- | --- |
| 1 | The login is Cloudflare Access with managed OAuth. LifeOS gets no OAuth code. | The spike showed that the Claude app and Claude Code do the OAuth flow of Access with no code in the server. The design doc planned a GitHub OAuth flow of about 4 hours. |
| 2 | One MCP listener. The server checks the token itself, although `cloudflared` checks the same token first. Each request needs a token, also a request from the Pi, so Claude Code on the Pi uses the public hostname. | `cloudflared` guards only what comes through the tunnel. The check in the server does three more things. It takes the place of the Host guard of the MCP library, which must be off for the tunnel; without a guard, a web page in a browser on the Pi can call the tools. It keeps the data closed when a change of the tunnel config is wrong. It lets no program on the Pi call the tools without a login. A second listener with no check is a second way in. The connector of claude.ai syncs to Claude Code with no second login. |
| 3 | No skill is uploaded. The seven drafts move to `lifeos-core/skills/` unchanged. | The rules are in the tool descriptions (plan 6). A draft becomes a skill only when the week of daily use shows a procedure that the agent does wrong. `lifeos-wiki` holds only the data of the user. |
| 4 | No systemd unit for `claude remote-control`. | The week of daily use shows if a session in the wiki folder has a use that the MCP tools do not cover. |
| 5 | The team name and the audience tag come from the environment (`CF_ACCESS_TEAM`, `LIFEOS_MCP_AUD` in `deploy/.env`), not from `config/lifeos.yaml`. | The two values are not secrets, but they name the Cloudflare account. The hostnames are in `deploy/.env` for the same reason. The other choice is two keys in the config file, which is in git. |
| 6 | The server answers 403, not 401, to a request that has no valid token. | Only Cloudflare Access answers 401. `deploy/check-access.sh` takes 401 as the proof that Access stopped a request, so the answer of the server must be different. |
| 7 | The Access application of the MCP hostname gets a second policy: it accepts each valid service token of the account, as the application of the API does. | Then `deploy/check-access.sh` can prove after each deploy that the full way works: the tunnel, the token check and the MCP server. A device that has a service token can call the REST API already, which reads and changes each record. The other choice is no second policy; then the check proves only that the hostname is closed. |
| 8 | The server keeps the public keys of the team in memory with no time limit. It gets the key set again only for a key ID that it does not know, one time each minute at most. | Cloudflare changes its key each six weeks and gives the new key a new ID. Each deploy starts the server again and empties the keys. A key that Cloudflare removes early stays trusted until the next start. |
| 9 | The wiki goes to GitHub each hour, from a bare mirror in `/var/backups/lifeos/wiki.git`. The job never forces. It sends a message to ntfy only when it fails. | The server can write the live repository, so a setting or a hook there can be the work of a request from the internet. A job that has the SSH key must not run git inside that repository. A fetch starts only `git upload-pack` there, which runs no hook. |
| 10 | Task 7 makes one commit by hand inside the live repository: it removes `AGENTS.md`, `CLAUDE.md`, `README.md`, `.agents/` and `prompts/`. It does this before the hostname points to the server, and it never happens again. | Until that moment, only local clients reached the MCP server, so the repository holds no hook and no setting from a stranger. Step 2 of Task 7 checks that without a git command. The other way, a new clone from GitHub in the place of the live repository, needs a stop of the server and three more steps. |
| 11 | `~/lifeos-wiki` becomes a copy for reading: `git pull` only. | GitHub gets commits only from the mirror. A commit from a different clone makes the push fail, because the job does not force. Decision 13 has the fix. |
| 12 | The design doc changes only in the sentences that this plan makes false: the login of the MCP server, the push of the wiki, and the place of the skill drafts and the prompts. | The design doc is the plan of record, not a copy of the code. `README.md` and `AGENTS.md` describe what is built. |
| 13 | When GitHub has a commit that the wiki on the Pi does not have, the Pi wins. `deploy/wiki-push-repair.sh` keeps the commit of GitHub on a branch `from-github-<time>` and puts the history of the Pi on `main`. Nothing merges. A person runs the script; the hourly job only names it in its failure message. | The server is the one writer of the wiki: a page that changes outside the server has no event, and `index.md` can become wrong. A merge must also run inside the live repository with the SSH key, which decision 9 forbids. The change is not lost: Gautam reads it from the branch and gives it to the agent. |

## Prerequisites

Step 1 of Task 1 checks these.

- Plan 6 is complete: `lifeos.service` is active, and the working tree of `main` is clean.
- The container `postgres-test` runs. The tests use it (port 5433, data in memory).
- `deploy/.env` has `POSTGRES_PASSWORD`, `LIFEOS_QUERY_PASSWORD`, `LIFEOS_ADMIN_HOST`, `LIFEOS_API_HOST`, `CF_ACCESS_CLIENT_ID` and `CF_ACCESS_CLIENT_SECRET`. Never print a value.
- Cloudflare has the Access application `LifeOS MCP` with managed OAuth, and `/etc/cloudflared/config.yml` has a rule for the hostname `mcp.<domain>` with an `access` part. The rule points to `127.0.0.1:8190`, where nothing listens. The spike made both.
- `~/lifeos-wiki` is clean and holds the commit `e7b59c3`, which has the skill drafts and the prompts.
- The live wiki is `/var/lib/lifeos/lifeos-wiki`. It has commits that GitHub does not have.

## Global Constraints

- **No name of the account in git.** Never write the domain, a hostname, the team name or an audience tag of Cloudflare into a file of this repository, also not into a test or a commit message. A step reads them from `deploy/.env` or from `/etc/cloudflared/config.yml`. Never print a value of `deploy/.env`.
- **No mode without the check.** The MCP handler has no option, no config key and no environment variable that turns the token check off.
- **No token in a file.** A test builds each token from its parts with a key that it makes (`internal/testaccess`). No file holds a token, also not a token that is not valid.
- **No answer repeats a token.** An error text and a log line give the reason, never the token or a part of it.
- **Adapters are thin.** The token check is HTTP code of the MCP adapter. It is in `internal/mcp`, not in `internal/service`.
- **Tests never touch `/var/lib/lifeos`, `~/lifeos-wiki` or the network.** A test of the token check uses a key set on a local test server. A case of `deploy/test.sh` makes its git repositories in its temporary folder.
- **Tests use the server `postgres-test`.** Run tests with `make test`, not `go test`. Run one test command at a time: the Pi stopped to respond on 2026-10-11 during a test run.
- **Generated code is not in git.** `internal/store/` (sqlc) and `*_templ.go` (templ) are in `.gitignore`. Never `git add` them.
- **No git command inside the live repository after Step 2 of Task 7.** Read its history through the mirror: `git --git-dir=/var/backups/lifeos/wiki.git log`.
- **Where to work.** Work in `/home/gautam/lifeos-core` on `main`. Do not make a branch or a worktree: `deploy/.env` is not in git and exists only in this checkout. Do not push.
- **Commits.** Conventional commit format, one concern for each commit, with the exact file lists in the steps.
- **Ask first.** Task 7 changes the real system: the live wiki, GitHub, an Access application, the tunnel and the config of Claude Code. It starts with a step that gets Gautam's OK. A restart of `cloudflared` drops `ssh.`, `sysinfo.` and `ntfy.` for some seconds.
- **Order.** The hostname points to the real MCP port only after the server with the token check runs there.

## Review Focus

No test of this plan exercises these. Each line names the step that checks it on the real system, or says that nothing does.

1. **A token that Cloudflare signs is not the token of the tests.** The tests sign with their own key and choose the claims. A real token can have the audience as one text or as a list, and more fields in its header. Step 8 of Task 7 sends a real token of a service through the full way; Task 8 does the same for the token of a person.
2. **The server cannot get the key set from inside its unit.** The unit limits what the server sees (`ProtectSystem`, `ProtectHome`). When it cannot reach `cloudflareaccess.com`, it answers 403 to each request, also to one with a valid token. Step 8 of Task 7 shows this as `status 403 with the service token`; the log of the server gives the reason `the key set did not come`.
3. **A request that goes past the path limit.** The tunnel rule sends only the path `/mcp`. Step 8 of Task 7 checks one other path with `cloudflared tunnel ingress rule`. Nothing checks each form of a path, such as `/mcp/` or `/MCP`.
4. **The clock of the Pi is wrong after a start.** The Pi has no clock battery. Until the time comes from the network, the clock is in the past, and the server accepts a token that has expired. `cloudflared` checks the same token before the server, with the same clock. Nothing in this plan checks this.
5. **The push job has no way to GitHub for a long time, or the wiki gets a very large commit.** The job then fails each hour and sends one message to ntfy each time. Nothing limits the size of what it fetches. Nothing in this plan checks this.

## File Structure

| File | Task | Purpose |
| --- | --- | --- |
| `internal/testaccess/testaccess.go` | 1 | Test helper: a key set on a test server, and tokens that it signs |
| `internal/mcp/access.go` | 1 | `AccessCheck`: `NewAccessCheck`, `Verify`, `Require` |
| `internal/mcp/access_test.go`, `internal/mcp/export_test.go` | 1 | Tests of the check; the clock of the check for tests |
| `cmd/lifeos/access.go` | 2 | `accessCheck()`: the check from `CF_ACCESS_TEAM` and `LIFEOS_MCP_AUD` |
| `cmd/lifeos/access_test.go` | 2 | Tests of the MCP listener with and without a token; the test helpers `mcpTransport` and `postMCP` |
| `cmd/lifeos/serve.go`, `internal/mcp/server.go` | 2 | `serveOn` and `Handler` get the check; the Host guard of the MCP library is off |
| `cmd/lifeos/*_test.go` (5 files) | 2 | Each MCP client of a test sends a token |
| `Makefile`, `deploy/.env.example`, `deploy/systemd/lifeos.service`, `deploy/test.sh` | 2, 3, 4 | The new keys of `deploy/.env`; the cases of the new scripts |
| `deploy/check-access.sh` | 3 | The checks of the MCP hostname |
| `deploy/wiki-push.sh`, `deploy/systemd/lifeos-wiki-push.service`, `deploy/systemd/lifeos-wiki-push.timer` | 4 | The push of the wiki to GitHub |
| `deploy/wiki-push-repair.sh` | 4 | The fix when GitHub has a commit that the wiki does not have |
| `skills/*/SKILL.md` (7), `skills/README.md`, `prompts/*.md` (3) | 5 | The drafts and the prompts from `lifeos-wiki` |
| `AGENTS.md`, `README.md`, `docs/design.md` | 6 | What is built, and the sentences of the design doc that this plan makes false |

## How to read the steps

A step that changes a file uses one of these forms. Do each form exactly.

- **Create `path`:** make the new file with the content of the code block.
- **In `path`, replace this: ... With this: ...** The first code block is in the file one time. Put the second code block in its place. When a step has more than one of these for a file, do them from the first to the last.

Go code uses tabs. Copy the code blocks with their tabs.

The full test run takes about 100 seconds. Give a command that runs it a time limit of 5 minutes.

---

### Task 1: The check of the Cloudflare Access token

**Files:**
- Create: `internal/testaccess/testaccess.go`
- Create: `internal/mcp/access.go`
- Create: `internal/mcp/export_test.go`
- Test: `internal/mcp/access_test.go`

**Interfaces:**
- Consumes: nothing from an earlier task.
- Produces, in package `mcp` (`internal/mcp`):
  - `func NewAccessCheck(issuer, audience string) *AccessCheck`. `issuer` is the URL of the team, `https://<team>.cloudflareaccess.com`, with no `/` at its end. `audience` is the audience tag of the Access application.
  - `func (c *AccessCheck) Verify(token string) error`. It returns nil only for a token that the team signed with RS256 for this application and that has not expired.
  - `func (c *AccessCheck) Require(next http.Handler) http.Handler`. It reads the header `Cf-Access-Jwt-Assertion` and answers 403 when `Verify` refuses.
  - `func (c *AccessCheck) SetNow(now func() time.Time)`, only for tests (`export_test.go`).
- Produces, in package `testaccess` (`internal/testaccess`), for tests only:
  - `func New(t testing.TB) *Access`, with the fields `Issuer` and `Audience`.
  - `func (a *Access) Token() string`, `Claims() map[string]any`, `Sign(claims map[string]any) string`, `KeyID() string`, `Rotate()`, `SetDown(down bool)`, `Fetches() int`.
  - `func Part(t testing.TB, value any) string` and the constant `Header`.

- [ ] **Step 1: Check the prerequisites**

Run:
```bash
cd /home/gautam/lifeos-core
systemctl is-active lifeos.service
docker inspect -f '{{.State.Health.Status}}' lifeos-postgres-test-1
git -C /home/gautam/lifeos-wiki cat-file -t e7b59c3
git status --short | grep -v 'docs/superpowers/plans/' | wc -l
git -C /home/gautam/lifeos-wiki status --short | wc -l
grep -c -E '^(POSTGRES_PASSWORD|LIFEOS_QUERY_PASSWORD|LIFEOS_ADMIN_HOST|LIFEOS_API_HOST|CF_ACCESS_CLIENT_ID|CF_ACCESS_CLIENT_SECRET)=.' deploy/.env
sudo grep -c '^    service: http://127.0.0.1:8190$' /etc/cloudflared/config.yml
```
Expected: the run passes. The output contains `active`, `healthy` and `commit`.

The four counts are `0`, `0`, `6` and `1`. Stop and tell Gautam when one is different.

- [ ] **Step 2: Write the test helper and the failing tests**

The helper stands in for Cloudflare Access: it serves a key set and signs tokens with the key of that set.

Create `internal/testaccess/testaccess.go`:

```go
// Package testaccess is a test helper. It stands in for Cloudflare Access:
// it serves a key set and signs tokens with the key of that set. Each
// token is built from its parts, so no file holds a token.
package testaccess

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// Header is the request header in which Cloudflare Access sends its token.
const Header = "Cf-Access-Jwt-Assertion"

// Access is one team of Cloudflare Access with one application.
type Access struct {
	// Issuer is the URL of the team. The key set is below it.
	Issuer string
	// Audience is the audience tag of the application.
	Audience string

	t  testing.TB
	mu sync.Mutex
	// key signs the tokens; kid is its ID in the key set.
	key     *rsa.PrivateKey
	kid     string
	fetches int
	down    bool
}

// New starts the server of the key set. The server stops when the test
// ends.
func New(t testing.TB) *Access {
	t.Helper()
	a := &Access{t: t, Audience: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"}
	a.Rotate()
	server := httptest.NewServer(http.HandlerFunc(a.serveKeys))
	t.Cleanup(server.Close)
	a.Issuer = server.URL
	return a
}

func (a *Access) serveKeys(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if r.URL.Path != "/cdn-cgi/access/certs" {
		http.NotFound(w, r)
		return
	}
	a.fetches++
	if a.down {
		http.Error(w, "the key set is not available", http.StatusServiceUnavailable)
		return
	}
	public := a.key.PublicKey
	json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]string{{
		"kid": a.kid, "kty": "RSA", "alg": "RS256", "use": "sig",
		"n": encode(public.N.Bytes()),
		"e": encode(big.NewInt(int64(public.E)).Bytes()),
	}}})
}

// Rotate replaces the key of the set. A token that the old key signed has
// a key ID that the set does not hold any more.
func (a *Access) Rotate() {
	a.t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		a.t.Fatal(err)
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.key = key
	a.kid = fmt.Sprintf("key-%d", time.Now().UnixNano())
}

// SetDown makes the server of the key set answer with an error, or work
// again.
func (a *Access) SetDown(down bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.down = down
}

// Fetches returns how many times a client asked for the key set.
func (a *Access) Fetches() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.fetches
}

// Claims returns the claims of a token that is valid for five minutes.
// A test changes one of them to make a token that the check must refuse.
func (a *Access) Claims() map[string]any {
	return map[string]any{
		"iss":   a.Issuer,
		"aud":   []string{a.Audience},
		"exp":   time.Now().Add(5 * time.Minute).Unix(),
		"iat":   time.Now().Unix(),
		"email": "person@example.com",
		"type":  "app",
	}
}

// Token returns a valid token.
func (a *Access) Token() string {
	a.t.Helper()
	return a.Sign(a.Claims())
}

// Sign returns a token that has these claims and the signature of the key
// of the set.
func (a *Access) Sign(claims map[string]any) string {
	a.t.Helper()
	a.mu.Lock()
	key, kid := a.key, a.kid
	a.mu.Unlock()
	signed := Part(a.t, map[string]any{"alg": "RS256", "kid": kid, "typ": "JWT"}) + "." + Part(a.t, claims)
	sum := sha256.Sum256([]byte(signed))
	signature, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, sum[:])
	if err != nil {
		a.t.Fatal(err)
	}
	return signed + "." + encode(signature)
}

// KeyID returns the ID of the key that signs the tokens now.
func (a *Access) KeyID() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.kid
}

// Part returns one part of a token: the JSON of value in the base64 form
// of a token.
func Part(t testing.TB, value any) string {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return encode(raw)
}

func encode(raw []byte) string { return base64.RawURLEncoding.EncodeToString(raw) }
```

Create `internal/mcp/export_test.go`:

```go
package mcp

import "time"

// SetNow replaces the clock of the check, so that a test does not wait one
// minute for the next fetch of the key set.
func (c *AccessCheck) SetNow(now func() time.Time) { c.now = now }
```

Create `internal/mcp/access_test.go`:

```go
package mcp_test

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	lifeosmcp "github.com/Gautam-J/lifeos-core/internal/mcp"
	"github.com/Gautam-J/lifeos-core/internal/testaccess"
)

func TestAccessCheckAcceptsATokenOfItsApplication(t *testing.T) {
	access := testaccess.New(t)
	check := lifeosmcp.NewAccessCheck(access.Issuer, access.Audience)

	if err := check.Verify(access.Token()); err != nil {
		t.Fatalf("Verify: %v, want a valid token to pass", err)
	}

	// A token of a service has the audience as one text, not as a list.
	claims := access.Claims()
	claims["aud"] = access.Audience
	if err := check.Verify(access.Sign(claims)); err != nil {
		t.Errorf("Verify with the audience as a text: %v, want it to pass", err)
	}
}

func TestAccessCheckRefusesATokenThatIsNotValid(t *testing.T) {
	access := testaccess.New(t)
	other := testaccess.New(t)
	check := lifeosmcp.NewAccessCheck(access.Issuer, access.Audience)
	// The first call gets the key set, so that each case below fails for
	// its own reason.
	if err := check.Verify(access.Token()); err != nil {
		t.Fatalf("Verify: %v", err)
	}

	with := func(key string, value any) string {
		claims := access.Claims()
		if value == nil {
			delete(claims, key)
		} else {
			claims[key] = value
		}
		return access.Sign(claims)
	}
	valid := access.Token()
	parts := strings.Split(valid, ".")

	// A token that a different key signed, with the key ID of this team.
	forged := strings.Split(other.Sign(access.Claims()), ".")
	forged[0] = parts[0]

	// The classic change of the algorithm: the public data as the secret
	// of an HMAC signature.
	hmacHeader := testaccess.Part(t, map[string]any{"alg": "HS256", "kid": access.KeyID()})
	mac := hmac.New(sha256.New, []byte(access.Issuer))
	mac.Write([]byte(hmacHeader + "." + parts[1]))
	hmacToken := hmacHeader + "." + parts[1] + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))

	cases := []struct {
		name, token, want string
	}{
		{"no token", "", "no token"},
		{"not three parts", parts[0] + "." + parts[1], "not a token"},
		{"a header that is not base64", "!!!." + parts[1] + "." + parts[2], "header"},
		{"claims that are not JSON", parts[0] + "." + base64.RawURLEncoding.EncodeToString([]byte("not json")) + "." + parts[2], "signature"},
		{"no signature", parts[0] + "." + parts[1] + ".", "signature"},
		{"the signature of a different key", strings.Join(forged, "."), "signature"},
		{"the claims of a different token", parts[0] + "." + strings.Split(with("email", "other@example.com"), ".")[1] + "." + parts[2], "signature"},
		{"the algorithm none", testaccess.Part(t, map[string]any{"alg": "none", "kid": access.KeyID()}) + "." + parts[1] + ".", "algorithm"},
		{"the algorithm HS256", hmacToken, "algorithm"},
		{"a different application", with("aud", []string{strings.Repeat("f", 64)}), "audience"},
		{"no audience", with("aud", nil), "audience"},
		{"an audience that is a number", with("aud", 7), "claims"},
		{"a different team", with("iss", "https://other.cloudflareaccess.com"), "issuer"},
		{"a time of expiry in the past", with("exp", time.Now().Add(-time.Second).Unix()), "expired"},
		{"no time of expiry", with("exp", nil), "expired"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := check.Verify(tc.token)
			if err == nil {
				t.Fatal("Verify passed, want a refusal")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %q, want it to name %q", err, tc.want)
			}
			if tc.token != "" && strings.Contains(err.Error(), tc.token) {
				t.Error("the error repeats the token")
			}
		})
	}
}

func TestAccessCheckGetsTheKeySetAgainForANewKey(t *testing.T) {
	access := testaccess.New(t)
	check := lifeosmcp.NewAccessCheck(access.Issuer, access.Audience)
	now := time.Now()
	check.SetNow(func() time.Time { return now })

	for range 3 {
		if err := check.Verify(access.Token()); err != nil {
			t.Fatalf("Verify: %v", err)
		}
	}
	if got := access.Fetches(); got != 1 {
		t.Fatalf("the check got the key set %d times for three tokens of one key, want 1", got)
	}

	// Cloudflare replaces its key. A token of the new key comes less than
	// one minute after the last fetch: the check does not ask again, so
	// that a caller cannot make it ask for each request.
	access.Rotate()
	for range 3 {
		if err := check.Verify(access.Token()); err == nil || !strings.Contains(err.Error(), "key") {
			t.Fatalf("Verify with a new key in the same minute: %v, want a refusal that names the key", err)
		}
	}
	if got := access.Fetches(); got != 1 {
		t.Errorf("the check got the key set %d times in one minute, want 1", got)
	}

	now = now.Add(61 * time.Second)
	if err := check.Verify(access.Token()); err != nil {
		t.Fatalf("Verify with the new key after one minute: %v, want it to pass", err)
	}
	if got := access.Fetches(); got != 2 {
		t.Errorf("the check got the key set %d times, want 2", got)
	}
}

func TestAccessCheckRefusesWhenTheKeySetDoesNotCome(t *testing.T) {
	access := testaccess.New(t)
	check := lifeosmcp.NewAccessCheck(access.Issuer, access.Audience)
	now := time.Now()
	check.SetNow(func() time.Time { return now })

	access.SetDown(true)
	if err := check.Verify(access.Token()); err == nil || !strings.Contains(err.Error(), "key set") {
		t.Fatalf("Verify with no key set: %v, want a refusal that names the key set", err)
	}

	// The check does not keep the failure for good.
	access.SetDown(false)
	now = now.Add(61 * time.Second)
	if err := check.Verify(access.Token()); err != nil {
		t.Errorf("Verify after the key set came back: %v, want it to pass", err)
	}
}

func TestAccessCheckLetsOnlyARequestWithATokenThrough(t *testing.T) {
	access := testaccess.New(t)
	check := lifeosmcp.NewAccessCheck(access.Issuer, access.Audience)
	calls := 0
	handler := check.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusNoContent)
	}))

	send := func(token string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodPost, "/mcp", nil)
		if token != "" {
			request.Header.Set(testaccess.Header, token)
		}
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		return response
	}

	claims := access.Claims()
	claims["aud"] = []string{strings.Repeat("f", 64)}
	for name, token := range map[string]string{"no token": "", "a token of a different application": access.Sign(claims)} {
		response := send(token)
		// 403 and not 401: only Cloudflare Access answers 401, and
		// deploy/check-access.sh takes 401 as the proof that Access
		// stopped the request.
		if response.Code != http.StatusForbidden {
			t.Errorf("%s: status %d, want 403", name, response.Code)
		}
		if token != "" && strings.Contains(response.Body.String(), token) {
			t.Errorf("%s: the answer repeats the token", name)
		}
	}
	if calls != 0 {
		t.Fatalf("the handler ran %d times for requests that have no valid token", calls)
	}

	if response := send(access.Token()); response.Code != http.StatusNoContent || calls != 1 {
		t.Errorf("a valid token: status %d and %d calls, want 204 and 1", response.Code, calls)
	}
}
```

- [ ] **Step 3: Run the tests to see them fail**

Run:
```bash
make test PKG=./internal/mcp ARGS="-run TestAccessCheck -count=1"
```
Expected: the run fails. The output contains `undefined: AccessCheck`.

- [ ] **Step 4: Write the check**

Create `internal/mcp/access.go`:

```go
package mcp

import (
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/big"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"
)

// accessHeader is the request header in which Cloudflare Access sends its
// token to the server.
const accessHeader = "Cf-Access-Jwt-Assertion"

// AccessCheck verifies the token of Cloudflare Access. Access is the login
// of the MCP server: it signs one token for each request that it lets
// through, and the token names the application. LifeOS has no login of its
// own, so a request that has no valid token came past Access and gets no
// answer.
type AccessCheck struct {
	// issuer is the URL of the team: https://<team>.cloudflareaccess.com.
	issuer string
	// audience is the audience tag of the Access application.
	audience string
	client   *http.Client
	now      func() time.Time

	mu sync.Mutex
	// keys holds the public keys of the team by key ID. Cloudflare
	// replaces its key each six weeks. A restart of the server empties
	// the map.
	keys    map[string]*rsa.PublicKey
	fetched time.Time
}

// NewAccessCheck returns a check for the tokens that the team at issuer
// signs for the application that has this audience tag.
func NewAccessCheck(issuer, audience string) *AccessCheck {
	return &AccessCheck{
		issuer:   issuer,
		audience: audience,
		client:   &http.Client{Timeout: 10 * time.Second},
		now:      time.Now,
	}
}

// Require returns a handler that sends only a request with a valid token
// to next. It answers each other request with 403. Cloudflare Access
// answers a request that has no login with 401 or with a redirect, so a
// 403 from here shows that a request came past Access.
func (c *AccessCheck) Require(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := c.Verify(r.Header.Get(accessHeader)); err != nil {
			slog.Warn("mcp: request refused", "reason", err.Error())
			http.Error(w, "no valid token of Cloudflare Access", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// audience is the aud claim of a token: one text, or a list of texts.
type audience []string

func (a *audience) UnmarshalJSON(data []byte) error {
	var one string
	if json.Unmarshal(data, &one) == nil {
		*a = audience{one}
		return nil
	}
	var list []string
	if err := json.Unmarshal(data, &list); err != nil {
		return err
	}
	*a = list
	return nil
}

// Verify returns nil for a token that the team signed for the application
// and that has not expired. The error says why a token is refused. It
// never repeats the token or a part of it.
func (c *AccessCheck) Verify(token string) error {
	if token == "" {
		return errors.New("the request has no token")
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return errors.New("the text is not a token")
	}
	var header struct{ Alg, Kid string }
	if err := decodePart(parts[0], &header); err != nil {
		return errors.New("the header of the token is not valid")
	}
	// Only RS256: a token that names a different algorithm must not
	// choose how the server checks it.
	if header.Alg != "RS256" {
		return errors.New("the algorithm of the token is not RS256")
	}
	key, err := c.key(header.Kid)
	if err != nil {
		return err
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return errors.New("the signature of the token is not valid")
	}
	sum := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if err := rsa.VerifyPKCS1v15(key, crypto.SHA256, sum[:], signature); err != nil {
		return errors.New("the signature of the token does not match")
	}
	var claims struct {
		Iss string   `json:"iss"`
		Aud audience `json:"aud"`
		Exp int64    `json:"exp"`
	}
	if err := decodePart(parts[1], &claims); err != nil {
		return errors.New("the claims of the token are not valid")
	}
	if claims.Iss != c.issuer {
		return errors.New("the issuer of the token is a different team")
	}
	if !slices.Contains(claims.Aud, c.audience) {
		return errors.New("the audience of the token is a different application")
	}
	if c.now().Unix() >= claims.Exp {
		return errors.New("the token has expired")
	}
	return nil
}

// decodePart reads one part of a token into value.
func decodePart(part string, value any) error {
	raw, err := base64.RawURLEncoding.DecodeString(part)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, value)
}

// key returns the public key that has this ID. It gets the key set of the
// team when it does not know the ID, one time each minute at most: a
// caller that sends tokens with new IDs must not make the server ask
// Cloudflare for each one.
func (c *AccessCheck) key(id string) (*rsa.PublicKey, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if key, ok := c.keys[id]; ok {
		return key, nil
	}
	if !c.fetched.IsZero() && c.now().Sub(c.fetched) < time.Minute {
		return nil, errors.New("the key of the token is not in the key set")
	}
	c.fetched = c.now()
	keys, err := c.fetchKeys()
	if err != nil {
		return nil, fmt.Errorf("the key set did not come: %w", err)
	}
	c.keys = keys
	if key, ok := c.keys[id]; ok {
		return key, nil
	}
	return nil, errors.New("the key of the token is not in the key set")
}

// fetchKeys gets the public keys of the team from Cloudflare.
func (c *AccessCheck) fetchKeys() (map[string]*rsa.PublicKey, error) {
	response, err := c.client.Get(c.issuer + "/cdn-cgi/access/certs")
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %d", response.StatusCode)
	}
	var set struct {
		Keys []struct{ Kid, Kty, N, E string } `json:"keys"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&set); err != nil {
		return nil, err
	}
	keys := map[string]*rsa.PublicKey{}
	for _, k := range set.Keys {
		n, errN := base64.RawURLEncoding.DecodeString(k.N)
		e, errE := base64.RawURLEncoding.DecodeString(k.E)
		if k.Kty != "RSA" || errN != nil || errE != nil || len(e) > 4 {
			continue
		}
		keys[k.Kid] = &rsa.PublicKey{N: new(big.Int).SetBytes(n), E: int(new(big.Int).SetBytes(e).Int64())}
	}
	if len(keys) == 0 {
		return nil, errors.New("it holds no RSA key")
	}
	return keys, nil
}
```

- [ ] **Step 5: Run the tests to see them pass**

Run:
```bash
make test PKG=./internal/mcp ARGS="-run TestAccessCheck -count=1"
```
Expected: the run passes. The output contains `ok    deploy/test.sh` and `lifeos-core/internal/mcp`.

- [ ] **Step 6: Commit**

Run:
```bash
git add internal/testaccess/testaccess.go internal/mcp/access.go internal/mcp/access_test.go internal/mcp/export_test.go
git commit -m "feat(mcp): add the check of the Cloudflare Access token"
```
Expected: the run passes.

---

### Task 2: The MCP server answers only a request that has a token

**Files:**
- Create: `cmd/lifeos/access.go`
- Modify: `cmd/lifeos/serve.go`, `internal/mcp/server.go`
- Modify: `Makefile`, `deploy/.env.example`, `deploy/systemd/lifeos.service`, `deploy/test.sh`
- Test: `cmd/lifeos/access_test.go` (new), `cmd/lifeos/lifecycle_test.go`, `cmd/lifeos/serve_test.go`, `cmd/lifeos/capture_test.go`, `cmd/lifeos/daily_test.go`, `cmd/lifeos/protect_test.go`

**Interfaces:**
- Consumes: `lifeosmcp.NewAccessCheck`, `(*AccessCheck).Require` and package `testaccess` from Task 1.
- Produces:
  - `func Handler(svc *service.Service, access *AccessCheck) http.Handler` in `internal/mcp`. It had one parameter before.
  - `func serveOn(ctx context.Context, httpListener, mcpListener net.Listener, svc *service.Service, access *lifeosmcp.AccessCheck) error` in `cmd/lifeos`.
  - `func accessCheck() (*lifeosmcp.AccessCheck, error)` in `cmd/lifeos`. It reads `CF_ACCESS_TEAM` and `LIFEOS_MCP_AUD`.
  - Test helpers in `cmd/lifeos`: `startServer(t) (config.Config, *pgxpool.Pool, *testaccess.Access)` (it had two results before), `mcpTransport(addr string, access *testaccess.Access) *sdk.StreamableClientTransport` and `postMCP(t, addr, host, token string) (int, string)`.
  - The keys `CF_ACCESS_TEAM` and `LIFEOS_MCP_AUD` of `deploy/.env`. The unit gives them to the server.

- [ ] **Step 1: Write the failing tests**

Each MCP client of a test must send a token now. `startServer` makes the stand-in for Cloudflare Access and returns it.

Create `cmd/lifeos/access_test.go`:

```go
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
```

In `cmd/lifeos/lifecycle_test.go`, replace this:

```go
	"github.com/Gautam-J/lifeos-core/internal/config"
	"github.com/Gautam-J/lifeos-core/internal/service"
	"github.com/Gautam-J/lifeos-core/internal/testdb"
```

With this:

```go
	"github.com/Gautam-J/lifeos-core/internal/config"
	lifeosmcp "github.com/Gautam-J/lifeos-core/internal/mcp"
	"github.com/Gautam-J/lifeos-core/internal/service"
	"github.com/Gautam-J/lifeos-core/internal/testaccess"
	"github.com/Gautam-J/lifeos-core/internal/testdb"
```

In `cmd/lifeos/lifecycle_test.go`, replace this:

```go
// startServer runs serveOn on free ports with a new database, a file store
// in a temporary folder and the read-only query pool. The servers stop when
// the test ends.
func startServer(t *testing.T) (config.Config, *pgxpool.Pool) {
	t.Helper()
```

With this:

```go
// startServer runs serveOn on free ports with a new database, a file store
// in a temporary folder, the read-only query pool and a stand-in for
// Cloudflare Access, which signs the tokens that the MCP server wants. The
// servers stop when the test ends.
func startServer(t *testing.T) (config.Config, *pgxpool.Pool, *testaccess.Access) {
	t.Helper()
```

In `cmd/lifeos/lifecycle_test.go`, replace this:

```go
	done := make(chan error, 1)
	go func() { done <- serveOn(ctx, httpListener, mcpListener, svc) }()
	t.Cleanup(func() {
```

With this:

```go
	done := make(chan error, 1)
	access := testaccess.New(t)
	go func() {
		done <- serveOn(ctx, httpListener, mcpListener, svc, lifeosmcp.NewAccessCheck(access.Issuer, access.Audience))
	}()
	t.Cleanup(func() {
```

In `cmd/lifeos/lifecycle_test.go`, replace this:

```go
	})
	return cfg, pool
}
```

With this:

```go
	})
	return cfg, pool, access
}
```

In `cmd/lifeos/lifecycle_test.go`, replace this:

```go
func TestServeTakesOneRecordThroughItsLife(t *testing.T) {
	cfg, pool := startServer(t)
	ctx := context.Background()
```

With this:

```go
func TestServeTakesOneRecordThroughItsLife(t *testing.T) {
	cfg, pool, access := startServer(t)
	ctx := context.Background()
```

In `cmd/lifeos/lifecycle_test.go`, replace this:

```go
	client := sdk.NewClient(&sdk.Implementation{Name: "test", Version: "0"}, nil)
	session, err := client.Connect(ctx, &sdk.StreamableClientTransport{Endpoint: "http://" + cfg.MCPAddr + "/mcp"}, nil)
	if err != nil {
```

With this:

```go
	client := sdk.NewClient(&sdk.Implementation{Name: "test", Version: "0"}, nil)
	session, err := client.Connect(ctx, mcpTransport(cfg.MCPAddr, access), nil)
	if err != nil {
```

In `cmd/lifeos/serve_test.go`, replace this:

```go
	"github.com/Gautam-J/lifeos-core/internal/config"
	"github.com/Gautam-J/lifeos-core/internal/service"
	"github.com/Gautam-J/lifeos-core/internal/testdb"
```

With this:

```go
	"github.com/Gautam-J/lifeos-core/internal/config"
	lifeosmcp "github.com/Gautam-J/lifeos-core/internal/mcp"
	"github.com/Gautam-J/lifeos-core/internal/service"
	"github.com/Gautam-J/lifeos-core/internal/testaccess"
	"github.com/Gautam-J/lifeos-core/internal/testdb"
```

In `cmd/lifeos/serve_test.go`, replace this:

```go
	done := make(chan error, 1)
	go func() { done <- serveOn(ctx, httpListener, mcpListener, service.New(pool)) }()
```

With this:

```go
	done := make(chan error, 1)
	access := testaccess.New(t)
	go func() {
		done <- serveOn(ctx, httpListener, mcpListener, service.New(pool), lifeosmcp.NewAccessCheck(access.Issuer, access.Audience))
	}()
```

In `cmd/lifeos/serve_test.go`, replace this:

```go
	client := sdk.NewClient(&sdk.Implementation{Name: "test", Version: "0"}, nil)
	session, err := client.Connect(ctx, &sdk.StreamableClientTransport{Endpoint: "http://" + cfg.MCPAddr + "/mcp"}, nil)
	if err != nil {
```

With this:

```go
	client := sdk.NewClient(&sdk.Implementation{Name: "test", Version: "0"}, nil)
	session, err := client.Connect(ctx, mcpTransport(cfg.MCPAddr, access), nil)
	if err != nil {
```

In `cmd/lifeos/capture_test.go`, replace this:

```go
func TestServeTakesACaptureThroughEveryAdapter(t *testing.T) {
	cfg, pool := startServer(t)
	ctx := context.Background()
```

With this:

```go
func TestServeTakesACaptureThroughEveryAdapter(t *testing.T) {
	cfg, pool, access := startServer(t)
	ctx := context.Background()
```

In `cmd/lifeos/capture_test.go`, replace this:

```go
	client := sdk.NewClient(&sdk.Implementation{Name: "test", Version: "0"}, nil)
	session, err := client.Connect(ctx, &sdk.StreamableClientTransport{Endpoint: "http://" + cfg.MCPAddr + "/mcp"}, nil)
	if err != nil {
```

With this:

```go
	client := sdk.NewClient(&sdk.Implementation{Name: "test", Version: "0"}, nil)
	session, err := client.Connect(ctx, mcpTransport(cfg.MCPAddr, access), nil)
	if err != nil {
```

In `cmd/lifeos/daily_test.go`, replace this:

```go
func TestServeTakesADayOfRecordsThroughEveryAdapter(t *testing.T) {
	cfg, pool := startServer(t)
	ctx := context.Background()
```

With this:

```go
func TestServeTakesADayOfRecordsThroughEveryAdapter(t *testing.T) {
	cfg, pool, access := startServer(t)
	ctx := context.Background()
```

In `cmd/lifeos/daily_test.go`, replace this:

```go
	client := sdk.NewClient(&sdk.Implementation{Name: "test", Version: "0"}, nil)
	session, err := client.Connect(ctx, &sdk.StreamableClientTransport{Endpoint: "http://" + cfg.MCPAddr + "/mcp"}, nil)
	if err != nil {
```

With this:

```go
	client := sdk.NewClient(&sdk.Implementation{Name: "test", Version: "0"}, nil)
	session, err := client.Connect(ctx, mcpTransport(cfg.MCPAddr, access), nil)
	if err != nil {
```

In `cmd/lifeos/protect_test.go`, replace this:

```go
func TestServeRefusesAPostFromAnotherSite(t *testing.T) {
	cfg, pool := startServer(t)
	rest := "http://" + cfg.HTTPAddr
```

With this:

```go
func TestServeRefusesAPostFromAnotherSite(t *testing.T) {
	cfg, pool, _ := startServer(t)
	rest := "http://" + cfg.HTTPAddr
```

- [ ] **Step 2: Run the tests to see them fail**

Run:
```bash
make test PKG=./cmd/lifeos ARGS=-count=1
```
Expected: the run fails. The output contains `undefined: accessCheck`.

- [ ] **Step 3: Put the check in front of the MCP server**

Create `cmd/lifeos/access.go`:

```go
package main

import (
	"fmt"
	"os"
	"regexp"

	lifeosmcp "github.com/Gautam-J/lifeos-core/internal/mcp"
)

var (
	accessTeamPattern = regexp.MustCompile(`^[a-z0-9-]+$`)
	accessAudPattern  = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

// accessCheck returns the check of the Cloudflare Access token for the MCP
// server. The team name comes from CF_ACCESS_TEAM and the audience tag of
// the Access application from LIFEOS_MCP_AUD (deploy/.env). The two values
// are not secrets, but they name the Cloudflare account, so they are not in
// the config file, which is in git. The server does not start without
// them: the MCP server has no mode that needs no token.
func accessCheck() (*lifeosmcp.AccessCheck, error) {
	team, aud := os.Getenv("CF_ACCESS_TEAM"), os.Getenv("LIFEOS_MCP_AUD")
	if !accessTeamPattern.MatchString(team) {
		return nil, fmt.Errorf("CF_ACCESS_TEAM: %q is not the team name of Cloudflare Access, such as my-team", team)
	}
	if !accessAudPattern.MatchString(aud) {
		return nil, fmt.Errorf("LIFEOS_MCP_AUD: the value is not an audience tag of an Access application, which has 64 characters from 0-9 and a-f")
	}
	return lifeosmcp.NewAccessCheck("https://"+team+".cloudflareaccess.com", aud), nil
}
```

In `cmd/lifeos/serve.go`, replace this:

```go
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
```

With this:

```go
		return err
	}
	access, err := accessCheck()
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
```

In `cmd/lifeos/serve.go`, replace this:

```go
	}
	slog.Info("lifeos is listening", "http", httpListener.Addr().String(), "mcp", mcpListener.Addr().String())
	return serveOn(ctx, httpListener, mcpListener, svc)
}
```

With this:

```go
	}
	slog.Info("lifeos is listening", "http", httpListener.Addr().String(), "mcp", mcpListener.Addr().String())
	return serveOn(ctx, httpListener, mcpListener, svc, access)
}
```

In `cmd/lifeos/serve.go`, replace this:

```go
// serveOn serves the REST API and the admin pages on httpListener and the
// MCP server on mcpListener. It returns nil after ctx is cancelled and the
// servers have shut down.
func serveOn(ctx context.Context, httpListener, mcpListener net.Listener, svc *service.Service) error {
	router := chi.NewRouter()
	api.Register(router, svc)
```

With this:

```go
// serveOn serves the REST API and the admin pages on httpListener and the
// MCP server on mcpListener. The MCP server answers only a request that
// has a token that access accepts. It returns nil after ctx is cancelled
// and the servers have shut down.
func serveOn(ctx context.Context, httpListener, mcpListener net.Listener, svc *service.Service, access *lifeosmcp.AccessCheck) error {
	router := chi.NewRouter()
	api.Register(router, svc)
```

In `cmd/lifeos/serve.go`, replace this:

```go
	mcpMux := http.NewServeMux()
	mcpMux.Handle("/mcp", lifeosmcp.Handler(svc))

	// Cloudflare Access is the login, and the browser sends that login
```

With this:

```go
	mcpMux := http.NewServeMux()
	mcpMux.Handle("/mcp", lifeosmcp.Handler(svc, access))

	// Cloudflare Access is the login, and the browser sends that login
```

In `internal/mcp/server.go`, replace this:

```go
// open, and that stream would block the shutdown of the HTTP server.
func Handler(svc *service.Service) http.Handler {
	server := NewServer(svc)
	return sdk.NewStreamableHTTPHandler(
		func(*http.Request) *sdk.Server { return server },
		&sdk.StreamableHTTPOptions{Stateless: true},
	)
}
```

With this:

```go
// open, and that stream would block the shutdown of the HTTP server.
func Handler(svc *service.Service, access *AccessCheck) http.Handler {
	server := NewServer(svc)
	return access.Require(sdk.NewStreamableHTTPHandler(
		func(*http.Request) *sdk.Server { return server },
		&sdk.StreamableHTTPOptions{Stateless: true},
	))
}
```

- [ ] **Step 4: Run the tests: one more failure**

The tunnel sends each request with the public hostname to the loopback listener. The MCP library refuses such a request.

Run:
```bash
make test PKG=./cmd/lifeos ARGS=-count=1
```
Expected: the run fails. The output contains `TestServeAnswersAnMCPRequestForThePublicHostname` and `invalid Host header`.

- [ ] **Step 5: Turn the Host guard of the MCP library off**

The token check is the guard now. Do this step only together with Step 3: without the check, this option opens the server to a web page in a browser on the Pi.

In `internal/mcp/server.go`, replace this:

```go
// open, and that stream would block the shutdown of the HTTP server.
func Handler(svc *service.Service, access *AccessCheck) http.Handler {
```

With this:

```go
// open, and that stream would block the shutdown of the HTTP server.
//
// Each request needs a token that access accepts. The MCP library refuses
// a request to a loopback address that names a different host, as a guard
// against a web page that reaches a local server. That guard is off: the
// tunnel sends each request with the public hostname, and no web page can
// make a token of Cloudflare Access.
func Handler(svc *service.Service, access *AccessCheck) http.Handler {
```

In `internal/mcp/server.go`, replace this:

```go
		func(*http.Request) *sdk.Server { return server },
		&sdk.StreamableHTTPOptions{Stateless: true},
	))
```

With this:

```go
		func(*http.Request) *sdk.Server { return server },
		&sdk.StreamableHTTPOptions{Stateless: true, DisableLocalhostProtection: true},
	))
```

- [ ] **Step 6: Run the tests to see them pass**

Run:
```bash
make test PKG=./cmd/lifeos ARGS=-count=1
```
Expected: the run passes. The output contains `lifeos-core/cmd/lifeos`.

- [ ] **Step 7: Name the two keys in the example of the env file**

In `deploy/.env.example`, replace this:

```
# deploy/.env is not in git. Generate a password with: openssl rand -hex 24
# The server needs only the two database passwords. For each other key that
# you add here, add its name to UnsetEnvironment in systemd/lifeos.service.
```

With this:

```
# deploy/.env is not in git. Generate a password with: openssl rand -hex 24
# The server needs only the two database passwords and the two values of
# Cloudflare Access for the MCP server. For each other key that you add
# here, add its name to UnsetEnvironment in systemd/lifeos.service.
```

In `deploy/.env.example`, replace this:

```
# Public hostnames of LifeOS (check-access.sh): the admin pages and the REST
```

With this:

```
# Cloudflare Access (lifeos serve): the MCP server answers only a request
# that has a token of the Access application of the MCP hostname. The team
# name is the first part of <team>.cloudflareaccess.com. The audience tag
# is on the page of the application in the Cloudflare dashboard. The server
# does not start without the two values. They are not secrets, but they
# name the Cloudflare account.
CF_ACCESS_TEAM=
LIFEOS_MCP_AUD=

# Public hostnames of LifeOS (check-access.sh): the admin pages and the REST
```

Run:
```bash
deploy/test.sh
```
Expected: the run fails. The output contains `UnsetEnvironment does not name CF_ACCESS_TEAM` and `UnsetEnvironment does not name LIFEOS_MCP_AUD`.

The check is correct to fail: it knows only the two passwords as keys that the server needs.

- [ ] **Step 8: Let the unit and `make run` give the two keys to the server**

In `deploy/test.sh`, replace this:

```bash
# The server gets only the two database passwords. Each other key of the
# env file must be in UnsetEnvironment of the unit.
unset_keys=" $(sed -n 's/^UnsetEnvironment=//p' systemd/lifeos.service | tr '\n' ' ') "
for key in $(sed -n 's/^\([A-Z][A-Z0-9_]*\)=.*/\1/p' .env.example); do
    case $key in POSTGRES_PASSWORD | LIFEOS_QUERY_PASSWORD) continue ;; esac
    if [[ $unset_keys != *" $key "* ]]; then
```

With this:

```bash
# The server gets only the two database passwords and the two values of
# Cloudflare Access for the token check of the MCP server. Each other key
# of the env file must be in UnsetEnvironment of the unit.
unset_keys=" $(sed -n 's/^UnsetEnvironment=//p' systemd/lifeos.service | tr '\n' ' ') "
for key in $(sed -n 's/^\([A-Z][A-Z0-9_]*\)=.*/\1/p' .env.example); do
    case $key in POSTGRES_PASSWORD | LIFEOS_QUERY_PASSWORD | CF_ACCESS_TEAM | LIFEOS_MCP_AUD) continue ;; esac
    if [[ $unset_keys != *" $key "* ]]; then
```

In `deploy/systemd/lifeos.service`, replace this:

```
# systemd reads KEY=value lines; it does not run the file as a shell does.
# The server needs only the two database passwords. Each other key of
# deploy/.env is in UnsetEnvironment; deploy/test.sh checks the list
# against deploy/.env.example.
EnvironmentFile=/home/gautam/lifeos-core/deploy/.env
```

With this:

```
# systemd reads KEY=value lines; it does not run the file as a shell does.
# The server needs only the two database passwords and, for the token
# check of the MCP server, the team name and the audience tag of Cloudflare
# Access (CF_ACCESS_TEAM, LIFEOS_MCP_AUD). Each other key of deploy/.env is
# in UnsetEnvironment; deploy/test.sh checks the list against
# deploy/.env.example.
EnvironmentFile=/home/gautam/lifeos-core/deploy/.env
```

In `Makefile`, replace this:

```makefile
# Reads the two database passwords from deploy/.env, which is not in git.
include deploy/.env
```

With this:

```makefile
# Reads the database passwords and the Access values from deploy/.env, which
# is not in git.
include deploy/.env
```

In `Makefile`, replace this:

```makefile
export LIFEOS_QUERY_PASSWORD
# Tests create and drop their own databases on the server postgres-test of
```

With this:

```makefile
export LIFEOS_QUERY_PASSWORD
# The team name and the audience tag of Cloudflare Access, for the token
# check of the MCP server. `make run` needs them.
export CF_ACCESS_TEAM
export LIFEOS_MCP_AUD
# Tests create and drop their own databases on the server postgres-test of
```

Run:
```bash
deploy/test.sh
```
Expected: the run passes. The output contains `ok    deploy/test.sh`.

- [ ] **Step 9: Run all tests**

Run:
```bash
make test ARGS=-count=1
```
Expected: the run passes. The output contains `lifeos-core/internal/service` and `lifeos-core/cmd/lifeos`.

- [ ] **Step 10: Commit**

Run:
```bash
git add cmd/lifeos/access.go cmd/lifeos/access_test.go cmd/lifeos/serve.go cmd/lifeos/serve_test.go cmd/lifeos/lifecycle_test.go cmd/lifeos/capture_test.go cmd/lifeos/daily_test.go cmd/lifeos/protect_test.go internal/mcp/server.go Makefile deploy/.env.example deploy/systemd/lifeos.service deploy/test.sh
git commit -m "feat(mcp): answer only a request that has a token of Cloudflare Access"
```
Expected: the run passes.

---

### Task 3: `check-access.sh` checks the MCP hostname

**Files:**
- Modify: `deploy/check-access.sh`, `deploy/.env.example`, `deploy/systemd/lifeos.service`
- Test: `deploy/test.sh`

**Interfaces:**
- Consumes: the answer 403 of the MCP server for a request that has no valid token (Task 2).
- Produces: the key `LIFEOS_MCP_HOST` of `deploy/.env`. With it, `deploy/check-access.sh` checks `https://$LIFEOS_MCP_HOST/mcp` and `https://$LIFEOS_MCP_HOST/`. Task 7 sets the key after the tunnel points to the server.

- [ ] **Step 1: Write the failing cases**

In `deploy/test.sh`, replace this:

```bash
LIFEOS_API_HOST=api.test
CF_ACCESS_CLIENT_ID=id
```

With this:

```bash
LIFEOS_API_HOST=api.test
LIFEOS_MCP_HOST=mcp.test
CF_ACCESS_CLIENT_ID=id
```

In `deploy/test.sh`, replace this:

```bash
# check <name> <want exit> <want text> <answers>: runs check-access.sh
# against the table of answers.
check() {
```

With this:

```bash
# check <name> <want exit> <want text> <answers>: runs check-access.sh
# against the table of answers, with the env file $hosts.
hosts=$tmp/hosts.env
check() {
```

In `deploy/test.sh`, replace this:

```bash
    printf '%s\n' "$4" > "$tmp/answers"
    output=$(ANSWERS="$tmp/answers" PATH="$tmp/bin:$PATH" ./check-access.sh "$tmp/hosts.env" 2>&1) || status=$?
    if [ "$status" != "$want_exit" ] || [[ $output != *"$want_text"* ]]; then
```

With this:

```bash
    printf '%s\n' "$4" > "$tmp/answers"
    output=$(ANSWERS="$tmp/answers" PATH="$tmp/bin:$PATH" ./check-access.sh "$hosts" 2>&1) || status=$?
    if [ "$status" != "$want_exit" ] || [[ $output != *"$want_text"* ]]; then
```

In `deploy/test.sh`, replace this:

```bash
https://api.test/api/items 401
token:https://api.test/api/items 200
token:https://api.test/admin/items 404"
```

With this:

```bash
https://api.test/api/items 401
https://mcp.test/mcp 401
https://mcp.test/ 401
token:https://api.test/api/items 200
token:https://api.test/admin/items 404
token:https://mcp.test/mcp 200"
```

In `deploy/test.sh`, replace this:

```bash
check "no answer" 1 "FAIL  https://admin.test/admin/items: no answer" "nothing 000"
```

With this:

```bash
check "no answer" 1 "FAIL  https://admin.test/admin/items: no answer" "nothing 000"

# The MCP hostname. The MCP server checks the token of Access itself and
# answers 403 to a request that has no valid token. So a 403 here shows
# that a request came past Access: the hostname is not guarded.
check "the MCP hostname is guarded" 0 "ok    https://mcp.test/mcp: Access refuses the request (401)" "$good"
check "the MCP server answers the service token" 0 "ok    https://mcp.test/mcp: status 200 with the service token" "$good"
check "the MCP hostname answers" 1 "OPEN  https://mcp.test/mcp" "${good/mcp.test\/mcp 401/mcp.test\/mcp 200}"
check "only the MCP server refuses" 1 "OPEN  https://mcp.test/mcp: status 403" "${good/mcp.test\/mcp 401/mcp.test\/mcp 403}"
check "a different path of the MCP hostname answers" 1 "OPEN  https://mcp.test/: status 404" "${good/mcp.test\/ 401/mcp.test\/ 404}"
check "the MCP server refuses the service token" 1 "FAIL  https://mcp.test/mcp: status 403 with the service token" "${good/token:https:\/\/mcp.test\/mcp 200/token:https:\/\/mcp.test\/mcp 403}"
# The MCP hostname alone is a hostname to check.
printf 'LIFEOS_MCP_HOST=mcp.test\n' > "$tmp/mcp.env"
hosts=$tmp/mcp.env
check "only the MCP hostname is set" 1 "OPEN  https://mcp.test/mcp" "https://mcp.test/mcp 200"
hosts=$tmp/hosts.env
```

- [ ] **Step 2: Run the cases to see them fail**

Run:
```bash
deploy/test.sh
```
Expected: the run fails. The output contains `FAIL  check-access: the MCP hostname is guarded` and `FAIL  check-access: only the MCP hostname is set`.

- [ ] **Step 3: Add the checks of the MCP hostname**

In `deploy/check-access.sh`, replace this:

```bash
# tunnel or of an Access application; deploy.sh runs it after each deploy.
#
# Needs LIFEOS_ADMIN_HOST and LIFEOS_API_HOST in deploy/.env. With
# CF_ACCESS_CLIENT_ID and CF_ACCESS_CLIENT_SECRET (an Access service token),
# it also checks that the API answers a request that has the token, and
# that the API hostname gives no admin page.
# Usage: check-access.sh [env-file]   (default: deploy/.env)
set -euo pipefail
```

With this:

```bash
# tunnel or of an Access application; deploy.sh runs it after each deploy.
#
# Needs LIFEOS_ADMIN_HOST, LIFEOS_API_HOST and LIFEOS_MCP_HOST in
# deploy/.env. With CF_ACCESS_CLIENT_ID and CF_ACCESS_CLIENT_SECRET (an
# Access service token), it also checks that the API and the MCP server
# answer a request that has the token, and that the API hostname gives no
# admin page.
# Usage: check-access.sh [env-file]   (default: deploy/.env)
set -euo pipefail
```

In `deploy/check-access.sh`, replace this:

```bash
set -a; . "$env_file"; set +a

if [ -z "${LIFEOS_ADMIN_HOST:-}" ] && [ -z "${LIFEOS_API_HOST:-}" ]; then
    echo "no public hostname is set in $env_file: nothing to check"
    exit 0
```

With this:

```bash
set -a; . "$env_file"; set +a

if [ -z "${LIFEOS_ADMIN_HOST:-}" ] && [ -z "${LIFEOS_API_HOST:-}" ] && [ -z "${LIFEOS_MCP_HOST:-}" ]; then
    echo "no public hostname is set in $env_file: nothing to check"
    exit 0
```

In `deploy/check-access.sh`, replace this:

```bash
}

# with_token <url> <status>: a request that has the service token must get
# this status from LifeOS.
with_token() {
    local url=$1 want=$2 code
    code=$(curl -s -o /dev/null --max-time 20 -w '%{http_code}' \
        -H "CF-Access-Client-Id: $CF_ACCESS_CLIENT_ID" \
        -H "CF-Access-Client-Secret: $CF_ACCESS_CLIENT_SECRET" "$url" || true)
    if [ "$code" = "$want" ]; then
        echo "ok    $url: status $code with the service token"
```

With this:

```bash
}

# with_token <url> <status> [curl option...]: a request that has the
# service token must get this status from LifeOS.
with_token() {
    local url=$1 want=$2 code
    shift 2
    code=$(curl -s -o /dev/null --max-time 20 -w '%{http_code}' \
        -H "CF-Access-Client-Id: $CF_ACCESS_CLIENT_ID" \
        -H "CF-Access-Client-Secret: $CF_ACCESS_CLIENT_SECRET" "$@" "$url" || true)
    if [ "$code" = "$want" ]; then
        echo "ok    $url: status $code with the service token"
```

In `deploy/check-access.sh`, replace this:

```bash
    fi
fi

exit "$failed"
```

With this:

```bash
    fi
fi
if [ -n "${LIFEOS_MCP_HOST:-}" ]; then
    # The MCP server checks the token of Access itself and answers 403 to a
    # request that has no valid token. A 403 here is thus no proof, as for
    # each hostname: it shows a request that came past Access.
    guarded token "https://$LIFEOS_MCP_HOST/mcp"
    # The tunnel sends only the path /mcp to the server. Access guards each
    # path of the hostname.
    guarded token "https://$LIFEOS_MCP_HOST/"
    if [ -n "${CF_ACCESS_CLIENT_ID:-}" ] && [ -n "${CF_ACCESS_CLIENT_SECRET:-}" ]; then
        # The first request of an MCP client. 200 shows the full way: the
        # tunnel, the token check of the server and the MCP server.
        with_token "https://$LIFEOS_MCP_HOST/mcp" 200 \
            -H 'Content-Type: application/json' -H 'Accept: application/json, text/event-stream' \
            -d '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"check-access","version":"1"}}}'
    fi
fi

exit "$failed"
```

In `deploy/.env.example`, replace this:

```
# Public hostnames of LifeOS (check-access.sh): the admin pages and the REST
# API, each behind its own Cloudflare Access application. Leave them empty
# when LifeOS has no public hostname.
LIFEOS_ADMIN_HOST=
LIFEOS_API_HOST=
# Cloudflare Access service token of the Pi (check-access.sh): lets the check
# call the API through the tunnel. Leave them empty to skip that part.
CF_ACCESS_CLIENT_ID=
```

With this:

```
# Public hostnames of LifeOS (check-access.sh): the admin pages, the REST
# API and the MCP server, each behind its own Cloudflare Access application.
# Leave them empty when LifeOS has no public hostname.
LIFEOS_ADMIN_HOST=
LIFEOS_API_HOST=
LIFEOS_MCP_HOST=
# Cloudflare Access service token of the Pi (check-access.sh): lets the check
# call the API and the MCP server through the tunnel. The Access application
# of each of the two must have a policy that accepts this token. Leave them
# empty to skip that part.
CF_ACCESS_CLIENT_ID=
```

Run:
```bash
deploy/test.sh
```
Expected: the run fails. The output contains `UnsetEnvironment does not name LIFEOS_MCP_HOST`.

Each case of `check-access` passes now. The line that is left is the check of the unit: the server does not need the new key.

- [ ] **Step 4: Keep the new key away from the server**

In `deploy/systemd/lifeos.service`, replace this:

```
UnsetEnvironment=CLOUDFLARE_ACCOUNT_ID CLOUDFLARE_API_TOKEN NTFY_BASE_URL NTFY_TOKEN
UnsetEnvironment=LIFEOS_ADMIN_HOST LIFEOS_API_HOST CF_ACCESS_CLIENT_ID CF_ACCESS_CLIENT_SECRET
ExecStart=/home/gautam/lifeos-core/bin/lifeos serve
```

With this:

```
UnsetEnvironment=CLOUDFLARE_ACCOUNT_ID CLOUDFLARE_API_TOKEN NTFY_BASE_URL NTFY_TOKEN
UnsetEnvironment=LIFEOS_ADMIN_HOST LIFEOS_API_HOST LIFEOS_MCP_HOST CF_ACCESS_CLIENT_ID CF_ACCESS_CLIENT_SECRET
ExecStart=/home/gautam/lifeos-core/bin/lifeos serve
```

Run:
```bash
deploy/test.sh
```
Expected: the run passes. The output contains `ok    deploy/test.sh`.

- [ ] **Step 5: Commit**

Run:
```bash
git add deploy/check-access.sh deploy/test.sh deploy/.env.example deploy/systemd/lifeos.service
git commit -m "feat(deploy): check that Cloudflare Access guards the MCP hostname"
```
Expected: the run passes.

---

### Task 4: The push of the wiki to GitHub

**Files:**
- Create: `deploy/wiki-push.sh`, `deploy/wiki-push-repair.sh`, `deploy/systemd/lifeos-wiki-push.service`, `deploy/systemd/lifeos-wiki-push.timer`
- Test: `deploy/test.sh`

**Interfaces:**
- Consumes: `wiki_dir` of `config/lifeos.yaml` and `ReadWritePaths` of `deploy/systemd/lifeos.service`, which `deploy/test.sh` reads already.
- Produces: `deploy/wiki-push.sh [live-repository [mirror]]`. The defaults are `/var/lib/lifeos/lifeos-wiki` and `/var/backups/lifeos/wiki.git`. It prints `GitHub has each commit of the wiki` or `pushed <commit>`, and it exits with 1 when a fetch or a push is refused. Task 7 makes the mirror and starts the timer.
- Produces: `deploy/wiki-push-repair.sh [mirror]`. When the remote has a commit that the mirror does not have, it pushes that commit to a branch `from-github-<time>` of the remote and then puts `main` of the mirror on `main` of the remote. In each other state it prints `nothing to repair` and changes nothing. Only a person runs it.

- [ ] **Step 1: Write the failing cases**

The cases make three repositories in the temporary folder: the remote, the live repository and the mirror. One case sets a trap in the live repository: settings and hooks that name a program. The script must not run that program. A control then shows that a git command inside the live repository does run it.

The last cases are for the repair. The remote gets a commit from a different clone, the push fails and names the repair script, and the repair keeps that commit on a branch.

In `deploy/test.sh`, replace this:

```bash
# The tests use the Postgres server postgres-test, whose data is in memory.
```

With this:

```bash
# wiki-push.sh copies the commits of the live wiki to the remote of the
# mirror. Here the remote, the live repository and the mirror are in the
# temporary folder, and no command uses the network.
w=$tmp/wiki
wgit() { git -c user.name=test -c user.email=test@example.com -c init.defaultBranch=main "$@"; }
# wiki_commit <text>: one more commit in the live repository, as the server
# makes one for each write of a page.
wiki_commit() {
    echo "$1" >> "$w/live/page.md"
    wgit -C "$w/live" add page.md
    wgit -C "$w/live" commit -q -m "$1"
}
# wiki_repos: the remote with one commit, the live repository with that
# commit, and the mirror as the header of wiki-push.sh says to make it.
wiki_repos() {
    rm -rf "$w"
    mkdir -p "$w"
    wgit init -q --bare "$w/remote.git"
    wgit init -q "$w/live"
    wiki_commit one
    git -C "$w/live" push -q "$w/remote.git" main
    wgit init -q --bare "$w/mirror.git"
    git -C "$w/mirror.git" remote add origin "$w/remote.git"
    git -C "$w/mirror.git" fetch -q origin
}
# push_check <name> <want exit> <want text>: runs wiki-push.sh.
push_check() {
    local name=$1 want_exit=$2 want_text=$3 output status=0
    output=$(./wiki-push.sh "$w/live" "$w/mirror.git" 2>&1) || status=$?
    if [ "$status" != "$want_exit" ] || [[ $output != *"$want_text"* ]]; then
        echo "FAIL  wiki-push: $name: exit $status, want $want_exit with the text '$want_text'"
        echo "$output" | sed 's/^/      /'
        failed=1
    fi
}
# remote_is <name> <commit>: the remote must be at this commit.
remote_is() {
    if [ "$(git -C "$w/remote.git" rev-parse main)" != "$2" ]; then
        echo "FAIL  wiki-push: $1: the remote is not at the commit $2"
        failed=1
    fi
}

wiki_repos
push_check "nothing new" 0 "GitHub has each commit of the wiki"
wiki_commit two
push_check "a new commit" 0 "pushed"
remote_is "a new commit" "$(git -C "$w/live" rev-parse HEAD)"
push_check "the same commit a second time" 0 "GitHub has each commit of the wiki"

# The server can write the live repository. A setting or a hook there must
# not make this script run a program. The trap is a program that leaves a
# file; each setting and each hook below names it.
wiki_commit three
printf '#!/bin/sh\ntouch %s\n' "$w/ran" > "$w/trap"
chmod +x "$w/trap"
mkdir "$w/hooks"
for hook in reference-transaction pre-auto-gc post-update pre-receive update post-receive; do
    cp "$w/trap" "$w/hooks/$hook"
done
for setting in uploadpack.packObjectsHook core.fsmonitor core.sshCommand core.pager core.alternateRefsCommand; do
    git -C "$w/live" config "$setting" "$w/trap"
done
git -C "$w/live" config core.hooksPath "$w/hooks"
push_check "settings and hooks in the live repository" 0 "pushed"
remote_is "settings and hooks in the live repository" "$(git --git-dir="$w/live/.git" rev-parse main)"
if [ -e "$w/ran" ]; then
    echo "FAIL  wiki-push: a setting or a hook of the live repository made the script run a program"
    failed=1
fi
# The control: a git command inside the live repository does run the trap.
# Without this, the case above proves nothing.
git -C "$w/live" status > /dev/null 2>&1 || true
if [ ! -e "$w/ran" ]; then
    echo "FAIL  wiki-push: the trap of this test does not work: git status inside the live repository did not run it"
    failed=1
fi

# The script never forces. A history that does not continue the last one
# stops it, and the remote keeps what it has.
wiki_repos
wiki_commit two
push_check "a new commit" 0 "pushed"
before=$(git -C "$w/remote.git" rev-parse main)
wgit -C "$w/live" commit -q --amend -m "two, changed"
push_check "the live wiki lost a commit" 1 "does not continue the history of the mirror"
remote_is "the live wiki lost a commit" "$before"
# The mirror keeps the commit that the live wiki lost.
if [ "$(git --git-dir="$w/mirror.git" rev-parse main)" != "$before" ]; then
    echo "FAIL  wiki-push: the live wiki lost a commit: the mirror did not keep the commit $before"
    failed=1
fi

wiki_repos
wgit clone -q "$w/remote.git" "$w/other"
echo other > "$w/other/other.md"
wgit -C "$w/other" add other.md
wgit -C "$w/other" commit -q -m other
git -C "$w/other" push -q origin main
before=$(git -C "$w/remote.git" rev-parse main)
wiki_commit two
push_check "the remote has a commit that the wiki does not have" 1 "deploy/wiki-push-repair.sh"
remote_is "the remote has a commit that the wiki does not have" "$before"

# wiki-push-repair.sh is the fix for that state. It keeps the commit of the
# remote on a branch there, and it puts the history of the wiki on main.
# repair_check <name> <want text>: runs wiki-push-repair.sh.
repair_check() {
    local name=$1 want_text=$2 output status=0
    output=$(./wiki-push-repair.sh "$w/mirror.git" 2>&1) || status=$?
    if [ "$status" != 0 ] || [[ $output != *"$want_text"* ]]; then
        echo "FAIL  wiki-push-repair: $name: exit $status, want 0 with the text '$want_text'"
        echo "$output" | sed 's/^/      /'
        failed=1
    fi
}
repair_check "the remote has a commit that the wiki does not have" "are on the branch from-github-"
remote_is "after the repair" "$(git -C "$w/live" rev-parse HEAD)"
kept=$(git -C "$w/remote.git" for-each-ref --format='%(objectname)' 'refs/heads/from-github-*')
if [ "$kept" != "$before" ]; then
    echo "FAIL  wiki-push-repair: the remote does not keep its commit $before on a branch from-github-*"
    failed=1
fi
push_check "after the repair" 0 "GitHub has each commit of the wiki"
wiki_commit three
push_check "a new commit after the repair" 0 "pushed"
# A second run finds nothing to do and makes no second branch.
repair_check "nothing to repair" "nothing to repair"
if [ "$(git -C "$w/remote.git" for-each-ref 'refs/heads/from-github-*' | wc -l)" != 1 ]; then
    echo "FAIL  wiki-push-repair: a run with nothing to repair made a branch"
    failed=1
fi

# The default paths of wiki-push.sh. The live repository holds wiki_dir.
# The mirror is in a folder that the server cannot write: the script runs
# git inside the mirror, with the SSH key of the user.
push_live=$(sed -n 's/^live=${1:-\(.*\)}$/\1/p' wiki-push.sh)
push_mirror=$(sed -n 's/^mirror=${2:-\(.*\)}$/\1/p' wiki-push.sh)
case $wiki_dir/ in
    "$push_live"/*) ;;
    *)
        echo "FAIL  wiki-push.sh: wiki_dir ($wiki_dir) is not in its live repository ($push_live)"
        failed=1
        ;;
esac
if [ -z "$push_mirror" ]; then
    echo "FAIL  wiki-push.sh: no default path of the mirror"
    failed=1
fi
for path in $(sed -n 's/^ReadWritePaths=//p' systemd/lifeos.service); do
    case $push_mirror/ in
        "$path"/*)
            echo "FAIL  wiki-push.sh: the server can write the mirror ($push_mirror)"
            failed=1
            ;;
    esac
done

# The tests use the Postgres server postgres-test, whose data is in memory.
```

- [ ] **Step 2: Run the cases to see them fail**

Run:
```bash
deploy/test.sh
```
Expected: the run fails. The output contains `FAIL  wiki-push: nothing new: exit 127` and `FAIL  wiki-push-repair: nothing to repair: exit 127`.

- [ ] **Step 3: Write the two scripts and the units**

Create `deploy/wiki-push.sh`:

```bash
#!/bin/bash
# Copies the commits of the live wiki to GitHub. The server makes one commit
# for each write of a page. It does not push: it has no key.
#
# The server can write the live repository, so a setting or a hook there
# can be the work of a request from the internet. This script runs no git
# command inside the live repository. It fetches from there into the
# mirror, a bare repository that the server cannot write, and it pushes
# from the mirror. For a fetch, git starts only `git upload-pack` in the
# source, and that program runs no hook and no program that a setting of
# the source names. deploy/test.sh holds a case for that.
#
# The script never forces. When the live wiki or GitHub has a history that
# does not continue the history of the mirror, it fails, and the unit sends
# the failure to ntfy. wiki-push-repair.sh is the fix for a commit that only
# GitHub has.
#
# One time, make the mirror:
#   git init --bare /var/backups/lifeos/wiki.git
#   git -C /var/backups/lifeos/wiki.git remote add origin <URL of the wiki repository>
#   git -C /var/backups/lifeos/wiki.git fetch origin
# Usage: wiki-push.sh [live-repository [mirror]]
set -euo pipefail

live=${1:-/var/lib/lifeos/lifeos-wiki}
mirror=${2:-/var/backups/lifeos/wiki.git}

# git gives no text when it refuses a fetch that is not a fast-forward.
if ! git --git-dir="$mirror" fetch --quiet --no-tags --no-recurse-submodules "$live" main:main; then
    echo "wiki-push: the fetch from $live failed. With no reason from git above, the live wiki does not continue the history of the mirror: a commit that the mirror has is not in the live wiki." >&2
    exit 1
fi
# The mirror knows what GitHub has from its last push. No change: no
# connection to GitHub.
if [ "$(git --git-dir="$mirror" rev-parse main)" = "$(git --git-dir="$mirror" rev-parse --quiet --verify refs/remotes/origin/main)" ]; then
    echo "GitHub has each commit of the wiki"
    exit 0
fi
if ! git --git-dir="$mirror" push --quiet origin main; then
    echo "wiki-push: GitHub refused the push. When GitHub has a commit that the wiki does not have, run deploy/wiki-push-repair.sh: it keeps that commit on a branch and puts the history of the wiki on main." >&2
    exit 1
fi
echo "pushed $(git --git-dir="$mirror" rev-parse --short main)"
```

Create `deploy/wiki-push-repair.sh`:

```bash
#!/bin/bash
# Repairs the push of the wiki after GitHub got a commit that the wiki on
# the Pi does not have: a change on github.com, or a push from a different
# clone. wiki-push.sh never forces, so it fails each hour in that state.
#
# The wiki on the Pi wins, because the server is the one writer of the wiki.
# This script loses nothing: it puts the commits that only GitHub has on a
# branch from-github-<time> on GitHub, and then it puts the history of the
# wiki on main. It changes main only when GitHub still has the commit that
# it put on the branch.
#
# After the repair, read the change and give it to the agent, which writes
# the page through the server. The script prints the command.
# Usage: wiki-push-repair.sh [mirror]
set -euo pipefail

mirror=${1:-/var/backups/lifeos/wiki.git}
g() { git --git-dir="$mirror" "$@"; }

g fetch --quiet --no-tags origin main
theirs=$(g rev-parse refs/remotes/origin/main)
if g merge-base --is-ancestor "$theirs" main; then
    echo "GitHub has no commit that the wiki does not have: nothing to repair"
    exit 0
fi

branch=from-github-$(date +%Y%m%d-%H%M%S)
g push --quiet origin "$theirs:refs/heads/$branch"
g push --quiet --force-with-lease="main:$theirs" origin main
echo "main on GitHub has the history of the wiki again."
echo "The commits that only GitHub had are on the branch $branch. To read them:"
echo "  git --git-dir=$mirror log -p main..$theirs"
```

Create `deploy/systemd/lifeos-wiki-push.service`:

```
[Unit]
Description=LifeOS wiki push (the commits of the wiki go to GitHub)
Wants=network-online.target
After=network-online.target
# No message on success: the unit runs each hour.
OnFailure=lifeos-notify@%n.service

[Service]
Type=oneshot
User=gautam
ExecStart=/home/gautam/lifeos-core/deploy/wiki-push.sh
```

Create `deploy/systemd/lifeos-wiki-push.timer`:

```
[Unit]
Description=Push the LifeOS wiki to GitHub each hour

[Timer]
OnCalendar=hourly
Persistent=true

[Install]
WantedBy=timers.target
```

- [ ] **Step 4: Run the cases to see them pass**

Run:
```bash
chmod +x deploy/wiki-push.sh deploy/wiki-push-repair.sh
deploy/test.sh
```
Expected: the run passes. The output contains `ok    deploy/test.sh`.

- [ ] **Step 5: Commit**

Run:
```bash
git add deploy/wiki-push.sh deploy/wiki-push-repair.sh deploy/systemd/lifeos-wiki-push.service deploy/systemd/lifeos-wiki-push.timer deploy/test.sh
git commit -m "feat(deploy): push the wiki to GitHub from a mirror that the server cannot write"
```
Expected: the run passes.

---

### Task 5: The skill drafts and the prompts move to this repository

**Files:**
- Create: `skills/ask/SKILL.md`, `skills/ingest/SKILL.md`, `skills/journal/SKILL.md`, `skills/lint/SKILL.md`, `skills/log/SKILL.md`, `skills/review/SKILL.md`, `skills/todo/SKILL.md` (copies)
- Create: `prompts/evening_brief.md`, `prompts/morning_brief.md`, `prompts/weekly_review.md` (copies)
- Create: `skills/README.md`

**Interfaces:**
- Consumes: the commit `e7b59c3` of `/home/gautam/lifeos-wiki`.
- Produces: the folders `skills/` and `prompts/`. No code reads them. Task 7 removes the files from the wiki repository.

The files hold no personal data: they name Gautam, as `AGENTS.md` does. Do not correct a draft in this task. `skills/README.md` says what is out of date.

- [ ] **Step 1: Copy the files from the commit of the wiki repository**

`git archive` reads the commit, so the result does not depend on the state of the folder `~/lifeos-wiki`.

Run:
```bash
mkdir -p skills
git -C /home/gautam/lifeos-wiki archive e7b59c3 prompts | tar -x
git -C /home/gautam/lifeos-wiki archive e7b59c3 .agents/skills | tar -x --strip-components=2 -C skills
find skills prompts -type f | sort | tr '\n' ' '
grep -r -l -i -E 'password|token|@[a-z]+\.' skills prompts | wc -l
```
Expected: the run passes. The output contains `prompts/evening_brief.md prompts/morning_brief.md prompts/weekly_review.md skills/ask/SKILL.md skills/ingest/SKILL.md skills/journal/SKILL.md skills/lint/SKILL.md skills/log/SKILL.md skills/review/SKILL.md skills/todo/SKILL.md`.

The count of the last line is `0`.

- [ ] **Step 2: Say what the drafts are**

Create `skills/README.md`:

```markdown
# Skill drafts

These seven files are drafts. No agent loads them.

They were written in the `lifeos-wiki` repository before the MCP tools existed. They are here unchanged, because `lifeos-wiki` holds only the data of the user.

The rules for an agent are in the tool descriptions, in the `next` field of the results of `journal` and `capture`, and in `internal/mcp/instructions.go` (rule 14 of [AGENTS.md](../AGENTS.md)). A draft becomes a skill only when daily use shows a procedure that the agent does wrong with those rules alone.

Correct a draft against the tools before you use it. Known differences:

- The marks `[planned]` and `[missing]` are out of date. Compare each tool with the tool list of the MCP server.
- The server writes `index.md`. An agent does not add a page there.
- A citation prefix is one of `WikiCitationPrefixes`. The drafts use prefixes that do not exist, such as `(visit:311)`.
- A record that an agent infers gets `dictated=false`.
```

- [ ] **Step 3: Commit**

Run:
```bash
git add skills prompts
git commit -m "docs: add the skill drafts and the brief prompts from the wiki repository"
```
Expected: the run passes.

---

### Task 6: Documentation

**Files:**
- Modify: `AGENTS.md`, `README.md`, `docs/design.md`

**Interfaces:**
- Consumes: the names of Tasks 1 to 5.
- Produces: nothing that code uses.

- [ ] **Step 1: Change the rules for work on the code**

The last bullet of the section "Deploy" said that the MCP listener has no public hostname. It becomes two bullets: the token check, and the push of the wiki.

In `AGENTS.md`, replace this:

```markdown
internal/testdb/       test helper: one migrated database for each test
internal/api/          chi + Huma handlers
```

With this:

```markdown
internal/testdb/       test helper: one migrated database for each test
internal/testaccess/   test helper: a stand-in for Cloudflare Access that signs tokens
internal/api/          chi + Huma handlers
```

In `AGENTS.md`, replace this:

```markdown
dashboards/            Grafana definitions, versioned with the schema they query
deploy/                docker-compose.yml, systemd units and timers, backup scripts, query-user.sh, postgres-test-init.sh, deploy.sh, check-access.sh, test.sh
docs/design.md         the design doc, exported
```

With this:

```markdown
dashboards/            Grafana definitions, versioned with the schema they query
deploy/                docker-compose.yml, systemd units and timers, backup scripts, query-user.sh, postgres-test-init.sh, deploy.sh, check-access.sh, wiki-push.sh, wiki-push-repair.sh, test.sh
skills/                drafts of agent skills; no agent loads them (skills/README.md)
prompts/               the prompts of the brief jobs (Phase 4)
docs/design.md         the design doc, exported
```

In `AGENTS.md`, replace this:

```markdown
`lifeos serve` runs the API and admin pages on `http_addr` and the MCP server on `mcp_addr`, path `/mcp`. On the Pi it runs as the systemd unit `deploy/systemd/lifeos.service`; `make run` is for work on the code. It needs the folders `files_dir`, `inbox_dir` and `wiki_dir` of the config file, the program `git`, and the two passwords `POSTGRES_PASSWORD` and `LIFEOS_QUERY_PASSWORD` in the environment. `lifeos migrate`, `lifeos job <name>` and `lifeos import-csv <importer> <file>` are run by hand or by systemd timers. Each command reads `config/lifeos.yaml`, or the file given with `-config`.
```

With this:

```markdown
`lifeos serve` runs the API and admin pages on `http_addr` and the MCP server on `mcp_addr`, path `/mcp`. On the Pi it runs as the systemd unit `deploy/systemd/lifeos.service`; `make run` is for work on the code. It needs the folders `files_dir`, `inbox_dir` and `wiki_dir` of the config file, the program `git`, and four values in the environment: the two passwords `POSTGRES_PASSWORD` and `LIFEOS_QUERY_PASSWORD`, and `CF_ACCESS_TEAM` and `LIFEOS_MCP_AUD` for the token check of the MCP server. `lifeos migrate`, `lifeos job <name>` and `lifeos import-csv <importer> <file>` are run by hand or by systemd timers. Each command reads `config/lifeos.yaml`, or the file given with `-config`.
```

In `AGENTS.md`, replace this:

```markdown
**LifeOS has no login of its own.** Cloudflare Access is the only lock on the two public hostnames: one for the admin pages (a login with an email code) and one for the REST API (an Access service token for each device). The names are in `deploy/.env`. So:

- Never add a hostname to the tunnel before its Access application exists.
- Run `deploy/check-access.sh` after each change of the tunnel or of an Access application. It fails when a hostname answers a request that has no login. Only an answer that Access alone gives is proof: the redirect to its login page, and 401 for the API hostname. `deploy/test.sh` holds the cases.
- Give each tunnel rule of a LifeOS hostname an `access` part: `required: true`, the team name and the audience tag of its Access application. Then `cloudflared` checks the token of Access itself, and the hostname stays closed when its application is removed.
- The unit gives the server only the two database passwords, the binary, the config and the folder `/var/lib/lifeos`, which holds the file store, the inbox and the repository of the wiki. Keep the wiki there: a process that can write a git repository can make git run a program for the next user of that repository, so the server must not write a repository in the home folder. Add each new key of `deploy/.env` to `UnsetEnvironment` in `deploy/systemd/lifeos.service`; `deploy/test.sh` checks the list against `deploy/.env.example`.
- `serve.go` puts `http.CrossOriginProtection` in front of the admin pages and the API. It refuses a request that changes data and that a browser sent from a different origin. Keep it: without it, a page of another site can use the Access login of the browser.
- The MCP listener has no public hostname. Give it one only after the MCP handler checks the token of Access itself (Phase 3).
```

With this:

```markdown
**LifeOS has no login of its own.** Cloudflare Access is the only lock on the three public hostnames: one for the admin pages (a login with an email code), one for the REST API (an Access service token for each device) and one for the MCP server (the OAuth login that Access gives to an MCP client). The names are in `deploy/.env`. So:

- Never add a hostname to the tunnel before its Access application exists.
- Run `deploy/check-access.sh` after each change of the tunnel or of an Access application. It fails when a hostname answers a request that has no login. Only an answer that Access alone gives is proof: the redirect to its login page, and 401 for the API hostname and the MCP hostname. `deploy/test.sh` holds the cases.
- Give each tunnel rule of a LifeOS hostname an `access` part: `required: true`, the team name and the audience tag of its Access application. Then `cloudflared` checks the token of Access itself, and the hostname stays closed when its application is removed.
- The unit gives the server only the two database passwords, the two Access values of the MCP server, the binary, the config and the folder `/var/lib/lifeos`, which holds the file store, the inbox and the repository of the wiki. Keep the wiki there: a process that can write a git repository can make git run a program for the next user of that repository, so the server must not write a repository in the home folder. Add each new key of `deploy/.env` to `UnsetEnvironment` in `deploy/systemd/lifeos.service`; `deploy/test.sh` checks the list against `deploy/.env.example`.
- `serve.go` puts `http.CrossOriginProtection` in front of the admin pages and the API. It refuses a request that changes data and that a browser sent from a different origin. Keep it: without it, a page of another site can use the Access login of the browser.
- **The MCP server checks the token of Access itself.** `AccessCheck` in `internal/mcp/access.go` answers 403 to each request that has no valid token of the Access application of the MCP hostname. The team name and the audience tag come from `CF_ACCESS_TEAM` and `LIFEOS_MCP_AUD`; the server does not start without them. Each client uses the public hostname, also Claude Code on the Pi: a request straight to `mcp_addr` has no token. The check replaces the Host guard of the MCP library, which is off because the tunnel sends the public hostname to a loopback address. So never turn the check off, and add no MCP route that does not go through `Handler`. The tunnel rule of the MCP hostname sends only the path `/mcp`.
- **The server does not push the wiki.** `deploy/wiki-push.sh` copies the commits to GitHub each hour (`lifeos-wiki-push.timer`). It fetches from the live repository into a mirror that the server cannot write, and it pushes from the mirror. Never run a git command inside the live repository as a user that has keys: the server can write that repository (see above). GitHub gets commits only from the mirror. A commit that reaches GitHub from a different place stops the push, and ntfy gets the failure. `deploy/wiki-push-repair.sh` is the fix: it keeps that commit on a branch `from-github-<time>` and puts the history of the Pi on `main`. It does not merge, because a merge is a write of the wiki that the server did not make (rule 14).
```

- [ ] **Step 2: Change the description of what is built**

In `README.md`, replace this:

```markdown
| 2 · Core data | Built: lists, journal, todos, habits, the review queue, capture, search, the read-only query, permanent delete, the three interfaces, the service unit and the public hostnames. The week of daily use is now the gate of Phase 3. Money tables and imports are in [docs/backlog.md](docs/backlog.md). |
| 3 · Agents and phone | In work. Built: the wiki tools. Not built: the public hostname of the MCP server and the connector of the Claude app. The gate is one week of daily use from the phone through the agent. |
| 4 to 5 | Not started. |
```

With this:

```markdown
| 2 · Core data | Built: lists, journal, todos, habits, the review queue, capture, search, the read-only query, permanent delete, the three interfaces, the service unit and the public hostnames. The week of daily use is now the gate of Phase 3. Money tables and imports are in [docs/backlog.md](docs/backlog.md). |
| 3 · Agents and phone | Built: the wiki tools, the public hostname of the MCP server with the login of Cloudflare Access, and the push of the wiki to GitHub. The gate is open: one week of daily use from the phone through the agent. |
| 4 to 5 | Not started. |
```

In `README.md`, replace this:

```markdown
- **Service.** `deploy/systemd/lifeos.service` starts `lifeos serve` at boot and starts it again after a failure. When it cannot start for about five minutes, ntfy gets a message. The unit gives the server only the two database passwords, the binary, the config and the file store: not the other values of `deploy/.env`, not the home folder and not the Docker socket. The logs are in the journal: `journalctl -u lifeos`.
- **Deploy.** `deploy/deploy.sh` pulls the code, builds it, dumps the database to `/var/backups/lifeos/deploy/`, applies the migrations, starts the service again and runs the check below.
- **Public hostnames.** The Cloudflare Tunnel gives the admin pages one hostname and the REST API a second one. Cloudflare Access guards each: a login with an email code for the admin pages, an Access service token for each device for the API. LifeOS has no login of its own, so Access is the only lock. `cloudflared` checks the token of Access on each request of the two hostnames, so a hostname stays closed when its Access application is removed.
- **Check.** `deploy/check-access.sh` fails when a hostname answers a request that has no login.
```

With this:

```markdown
- **Service.** `deploy/systemd/lifeos.service` starts `lifeos serve` at boot and starts it again after a failure. When it cannot start for about five minutes, ntfy gets a message. The unit gives the server only the two database passwords, the two Access values of the MCP server, the binary, the config and the folder of the file store and the wiki: not the other values of `deploy/.env`, not the home folder and not the Docker socket. The logs are in the journal: `journalctl -u lifeos`.
- **Deploy.** `deploy/deploy.sh` pulls the code, builds it, dumps the database to `/var/backups/lifeos/deploy/`, applies the migrations, starts the service again and runs the check below.
- **Public hostnames.** The Cloudflare Tunnel gives the admin pages one hostname, the REST API a second one and the MCP server a third one. Cloudflare Access guards each: a login with an email code for the admin pages, an Access service token for each device for the API, and an OAuth login for an MCP client. LifeOS has no login of its own, so Access is the only lock. `cloudflared` checks the token of Access on each request of the three hostnames, so a hostname stays closed when its Access application is removed.
- **MCP server.** The MCP server also checks the token of Access itself. It answers 403 to each request that has no valid token, also to a request from the Pi. The agent in the Claude app and Claude Code on the Pi use the same hostname.
- **Check.** `deploy/check-access.sh` fails when a hostname answers a request that has no login.
```

In `README.md`, replace this:

```markdown
- **History.** The git history of the wiki keeps each version of each page, also of a deleted page.
```

With this:

```markdown
- **History.** The git history of the wiki keeps each version of each page, also of a deleted page.
- **Copy on GitHub.** Each hour, `deploy/wiki-push.sh` copies the new commits to the private GitHub repository of the wiki. The server has no key and does not push. A clone of that repository on a different computer is for reading. A commit from there, or a change on github.com, stops the push, and ntfy gets a message. Then run `deploy/wiki-push-repair.sh`: it keeps your commit on a branch `from-github-<time>` on GitHub and puts the history of the Pi on `main`. Give the change to the agent after that, so that the server writes it.
```

In `README.md`, replace this:

```markdown
LifeOS has no login of its own. The admin pages and the REST API are public only through the Cloudflare Tunnel, each behind Cloudflare Access (section 13). The MCP listener has no public hostname.
```

With this:

```markdown
LifeOS has no login of its own. The admin pages, the REST API and the MCP server are public only through the Cloudflare Tunnel, each behind Cloudflare Access (section 13). The MCP server answers only a request that has a token of Access, so a client uses the public hostname and not the listener.
```

In `README.md`, replace this:

```markdown
A record that an MCP tool writes has the source `agent`. It is verified only when the tool input says that you dictated it.
```

With this:

```markdown
To connect the Claude app, add a custom connector at <https://claude.ai/customize/connectors> with the URL `https://<LIFEOS_MCP_HOST>/mcp`, and log in with the email code of Cloudflare Access. Claude Code that has the same claude.ai login gets the connector with no second login.

A record that an MCP tool writes has the source `agent`. It is verified only when the tool input says that you dictated it.
```

In `README.md`, replace this:

```markdown
You need Go 1.26, Docker, and a `deploy/.env` file with `POSTGRES_PASSWORD`, `LIFEOS_QUERY_PASSWORD` and `NTFY_BASE_URL`. The file is not in git. The backup scripts also need `RESTIC_PASSWORD`, the B2 settings and `NTFY_TOKEN`. You also need the two folders of the file store, `files_dir` and `inbox_dir` in `config/lifeos.yaml`, and the wiki folder `wiki_dir`, which is in a clone of the wiki repository.
```

With this:

```markdown
You need Go 1.26, Docker, and a `deploy/.env` file with `POSTGRES_PASSWORD`, `LIFEOS_QUERY_PASSWORD`, `NTFY_BASE_URL`, and `CF_ACCESS_TEAM` and `LIFEOS_MCP_AUD` for the token check of the MCP server. The file is not in git. The backup scripts also need `RESTIC_PASSWORD`, the B2 settings and `NTFY_TOKEN`. You also need the two folders of the file store, `files_dir` and `inbox_dir` in `config/lifeos.yaml`, and the wiki folder `wiki_dir`, which is in a clone of the wiki repository.
```

In `README.md`, replace this:

```markdown
The unit file holds the path `/home/gautam/lifeos-core` and the user `gautam`.
```

With this:

````markdown
The push of the wiki to GitHub needs a mirror, one time, and its timer:

```sh
git init --bare /var/backups/lifeos/wiki.git
git -C /var/backups/lifeos/wiki.git remote add origin git@github.com:Gautam-J/lifeos-wiki.git
git -C /var/backups/lifeos/wiki.git fetch origin
sudo systemctl link "$PWD/deploy/systemd/lifeos-wiki-push.service" "$PWD/deploy/systemd/lifeos-wiki-push.timer"
sudo systemctl enable --now lifeos-wiki-push.timer
```

The unit file holds the path `/home/gautam/lifeos-core` and the user `gautam`.
````

In `README.md`, replace this:

```markdown
Settings are in [config/lifeos.yaml](config/lifeos.yaml): the two listen addresses, the two database URLs, the file store and the wiki folder. The passwords come from `POSTGRES_PASSWORD` and `LIFEOS_QUERY_PASSWORD` in the environment.
```

With this:

```markdown
Settings are in [config/lifeos.yaml](config/lifeos.yaml): the two listen addresses, the two database URLs, the file store and the wiki folder. The passwords come from `POSTGRES_PASSWORD` and `LIFEOS_QUERY_PASSWORD` in the environment, and the team name and the audience tag of Cloudflare Access from `CF_ACCESS_TEAM` and `LIFEOS_MCP_AUD`.
```

In `README.md`, replace this:

```markdown
- **Money tables, the reconciliation with a bank statement and the one-time imports.** They are in [docs/backlog.md](docs/backlog.md). `lifeos import-csv` and `lifeos job` return "not implemented".
- **A push of the wiki to GitHub.** Each write of a page is a commit on the Pi. The push comes with the public hostname of the MCP server.
- **A public hostname for the MCP server.** It comes in Phase 3, with a login through Cloudflare Access and a check of the Access token in `lifeos`.
- **A way to get a deleted record back.** The page `/admin/deleted` only removes.
```

With this:

```markdown
- **Money tables, the reconciliation with a bank statement and the one-time imports.** They are in [docs/backlog.md](docs/backlog.md). `lifeos import-csv` and `lifeos job` return "not implemented".
- **A way to get a deleted record back.** The page `/admin/deleted` only removes.
```

In `README.md`, replace this:

```markdown
internal/testdb/     one migrated database for each test
queries/             SQL that sqlc compiles into internal/store/
migrations/          goose SQL migrations
deploy/              docker-compose.yml, backup scripts, query-user.sh, deploy.sh, check-access.sh, systemd units
docs/                design doc, backlog, implementation plans
```

With this:

```markdown
internal/testdb/     one migrated database for each test
internal/testaccess/ a stand-in for Cloudflare Access, for tests
queries/             SQL that sqlc compiles into internal/store/
migrations/          goose SQL migrations
deploy/              docker-compose.yml, backup scripts, query-user.sh, deploy.sh, check-access.sh, wiki-push.sh, wiki-push-repair.sh, systemd units
skills/              drafts of agent skills; no agent loads them
prompts/             the prompts of the brief jobs
docs/                design doc, backlog, implementation plans
```

- [ ] **Step 3: Change the sentences of the design doc that are false now**

In `docs/design.md`, replace this:

```markdown
Every agent uses one MCP endpoint, `mcp.<domain>`, with OAuth 2.1 allowlisted to Gautam's GitHub account; the REST API serves every client that is not an agent (iOS Shortcuts, Health Auto Export, OwnTracks, the Kindle dashboard, future apps).
```

With this:

```markdown
Every agent uses one MCP endpoint, `mcp.<domain>`, behind Cloudflare Access, which gives an MCP client an OAuth 2.1 login that only Gautam passes; the REST API serves every client that is not an agent (iOS Shortcuts, Health Auto Export, OwnTracks, the Kindle dashboard, future apps).
```

In `docs/design.md`, replace this:

```markdown
The Go SDK supplies OAuth building blocks (token checks, protected-resource metadata) rather than a ready GitHub sign-in, so Phase 3 includes about 4 hours to wire the GitHub OAuth flow, or to put a small OIDC provider in front of the MCP server.
```

With this:

```markdown
The server holds no OAuth code. Cloudflare Access does the OAuth flow for the MCP client (managed OAuth) and adds a signed token to each request that it lets through. The MCP server checks that token and answers no request without it.
```

In `docs/design.md`, replace this:

```markdown
- `CLAUDE.md`, `GEMINI.md`: one line each importing that repo's `AGENTS.md`, plus only what that tool needs differently.
- `lifeos-wiki/.agents/skills/<name>/SKILL.md`: the procedures, and the master copy. Skills call MCP tools; they never touch Postgres or wiki files directly.
- Each skill folder is zipped and uploaded once to claude.ai (Customize > Skills), which loads it in the Claude app and in Claude Code on the Pi. Other agents read `.agents/skills/` from the repo. No `.claude/skills` link on the Pi, so Claude Code does not see each skill twice. Re-upload after editing a skill.
- `lifeos-wiki/.claude/settings.json` denies Edit and Write on `wiki/**`: sessions read pages directly but write only through `wiki_write`.
```

With this:

```markdown
- `CLAUDE.md`, `GEMINI.md`: one line each importing that repo's `AGENTS.md`, plus only what that tool needs differently.
- `lifeos-core/skills/<name>/SKILL.md`: drafts of the procedures. No agent loads them: the rules are in the tool descriptions of the MCP server, and a draft becomes a skill only when daily use shows that a procedure needs one. `lifeos-wiki` holds only the wiki pages.
- When a draft becomes a skill, its folder is zipped and uploaded once to claude.ai (Customize > Skills), which loads it in the Claude app and in Claude Code on the Pi. Upload it again after each change.
- `lifeos-wiki/.claude/settings.json` denies Edit and Write on `wiki/**`: sessions read pages directly but write only through `wiki_write`.
```

In `docs/design.md`, replace this:

```markdown
Agent jobs (inbox processing, wiki lint) run `claude -p "/<skill>"` from the `lifeos-wiki` folder instead, signed in with the Pro login so the synced connector and skills load, with only the `lifeos` MCP tools allowed. Brief prompts live in `lifeos-wiki/prompts/`, so editing one changes the next run without a deploy.
```

With this:

```markdown
Agent jobs (inbox processing, wiki lint) run `claude -p "/<skill>"` from the `lifeos-wiki` folder instead, signed in with the Pro login so the synced connector and skills load, with only the `lifeos` MCP tools allowed. Brief prompts live in `lifeos-core/prompts/`.
```

In `docs/design.md`, replace this:

```markdown
| `api.` | lifeos `:8000`, `/api` only | Access service token in request headers | Per-device API token |
| `mcp.` | lifeos MCP `:8100` | None: Anthropic's servers must reach it; rate limit added | OAuth 2.1, only Gautam's GitHub account |
| `ntfy.` | ntfy `:8080` | None: the iOS app cannot do a browser login | ntfy users, everyone else denied |
```

With this:

```markdown
| `api.` | lifeos `:8000`, `/api` only | Access service token in request headers | Per-device API token |
| `mcp.` | lifeos MCP `:8100`, `/mcp` only | Access login through OAuth (managed OAuth), which the Claude app and Claude Code can do | The server checks the token of Access |
| `ntfy.` | ntfy `:8080` | None: the iOS app cannot do a browser login | ntfy users, everyone else denied |
```

In `docs/design.md`, replace this:

```markdown
Backups: the wiki is pushed to a private GitHub repo after each commit. Postgres dumps and the file store go nightly, encrypted by restic, to Backblaze B2; GitHub never holds the database. A restore is tested once a month.
```

With this:

```markdown
Backups: the wiki is pushed to a private GitHub repo each hour, by a job that is not the server. Postgres dumps and the file store go nightly, encrypted by restic, to Backblaze B2; GitHub never holds the database. A restore is tested once a month.
```

- [ ] **Step 4: Check that no file names the account, and run all tests**

Run:
```bash
domain=$(sed -n 's/^LIFEOS_ADMIN_HOST=[^.]*\.//p' deploy/.env)
team=$(sudo sed -n 's/^ *teamName: *//p' /etc/cloudflared/config.yml | head -1)
echo "names found: $(git grep --untracked -l -F -e "$domain" -e "$team" | wc -l)"
make test ARGS=-count=1
```
Expected: the run passes. The output contains `names found: 0`, `lifeos-core/internal/service` and `lifeos-core/cmd/lifeos`.

- [ ] **Step 5: Commit**

Run:
```bash
git add AGENTS.md README.md docs/design.md
git commit -m "docs: describe the MCP hostname, the token check and the push of the wiki"
```
Expected: the run passes.

---

### Task 7: Open the hostname on the Pi

This task changes the real system. Do the steps in this order: the order keeps the MCP server closed until its token check runs. Never print a value of `deploy/.env`, the domain, the team name or an audience tag.

**Interfaces:**
- Consumes: the commits of Tasks 1 to 6 on `main`.
- Produces: the MCP server at `https://<LIFEOS_MCP_HOST>/mcp`, the mirror `/var/backups/lifeos/wiki.git`, the timer `lifeos-wiki-push.timer`, and a wiki repository that holds only `wiki/` and `.claude/settings.json`.

- [ ] **Step 1: Get Gautam's OK**

Tell Gautam what this task does, then wait for his answer:

- It makes one commit by hand in the live wiki. The commit removes `AGENTS.md`, `CLAUDE.md`, `README.md`, `.agents/` and `prompts/`. The pages in `wiki/` do not change.
- It pushes the commits of the live wiki to GitHub, and it updates `~/lifeos-wiki` with `git pull`.
- It adds three keys to `deploy/.env`: `CF_ACCESS_TEAM`, `LIFEOS_MCP_AUD` and `LIFEOS_MCP_HOST`.
- It adds one policy to the Access application `LifeOS MCP`: each valid service token of the account can call the MCP server (decision 7).
- It runs `deploy/deploy.sh`. The admin pages and the API do not answer for some seconds. From then, the MCP tools of Claude Code on the Pi do not work until Gautam adds the connector (Task 8).
- It changes the tunnel rule of the MCP hostname and starts `cloudflared` again. `ssh.`, `sysinfo.` and `ntfy.` drop for some seconds.
- It starts a timer that pushes the wiki each hour.
- It removes the MCP entry `lifeos` from the config of Claude Code for this folder.

- [ ] **Step 2: Remove the files that are not wiki pages from the live wiki**

This is the one time that a git command runs inside the live repository with the rights of the user. It is safe only now: no client from the internet has reached the MCP server. The first two checks read files and run no git. `set -e` stops the block at the first check that fails, before the commit.

Run:
```bash
set -e
live=/var/lib/lifeos/lifeos-wiki
[ "$(ls "$live/.git/hooks" | grep -v -c '\.sample$' || true)" = 0 ]
[ "$(grep -v -c -E '^\[(core|remote "origin"|branch "main")\]$|^[[:space:]]+(repositoryformatversion|filemode|bare|logallrefupdates|url|fetch|remote|merge) = ' "$live/.git/config" || true)" = 0 ]
[ -z "$(git -C "$live" status --porcelain)" ]
git -C "$live" rm -r -q AGENTS.md CLAUDE.md README.md .agents prompts
git -C "$live" commit -q -m "chore: keep only the wiki pages and the Claude settings"
git -C "$live" ls-files | grep -v '^wiki/'
git -C "$live" log --format='%an: %s' -1
```
Expected: the run passes. The output contains `.claude/settings.json` and `chore: keep only the wiki pages and the Claude settings`.

The three checks are: no hook, no setting that a clone does not have, and no change that waits for a commit. When the block stops at one of them, do not continue: tell Gautam. `.claude/settings.json` is the only file that is not below `wiki/`.

From here, run no git command inside `/var/lib/lifeos/lifeos-wiki`.

- [ ] **Step 3: Make the mirror and push the wiki**

The push script refuses the commit of Step 2, because it changes files that are not wiki pages. That refusal is correct, and it shows that the guard works. This one commit goes to GitHub by hand.

Run:
```bash
set -e
cd /home/gautam/lifeos-core
mirror=/var/backups/lifeos/wiki.git
[ ! -e "$mirror" ]
git init -q --bare "$mirror"
git -C "$mirror" remote add origin "$(git -C /home/gautam/lifeos-wiki remote get-url origin)"
git -C "$mirror" fetch -q origin
deploy/wiki-push.sh || echo "the script refused: exit $?"
git --git-dir="$mirror" log --format='%an: %s' origin/main..main
git --git-dir="$mirror" push -q origin main
deploy/wiki-push.sh
git -C /home/gautam/lifeos-wiki pull -q --ff-only
ls -A /home/gautam/lifeos-wiki | tr '\n' ' '
```
Expected: the run passes. The output contains `a new commit changes a file that is not a wiki page`, `the script refused: exit 1`, `chore: keep only the wiki pages and the Claude settings`, `LifeOS: wiki:` and `GitHub has each commit of the wiki`.

The last line is `.claude .git wiki`. The list of `git log` has only the commit of Step 2 and commits of `LifeOS`.

- [ ] **Step 4: Put the team name and the audience tag into `deploy/.env`**

The tunnel config has the two values in the rule of the MCP hostname. The step checks them before it writes them: the form of each value, the team name against the login page that Access shows for the admin hostname, and the tag against the application `LifeOS MCP` at Cloudflare. It prints no value. The API token goes to `curl` through a file descriptor, not on the command line.

Run:
```bash
set -e
cd /home/gautam/lifeos-core
block=$(sudo awk '/^  - hostname: mcp\./{on=1; print; next} on && /^  - /{on=0} on' /etc/cloudflared/config.yml)
team=$(sed -n 's/^ *teamName: *//p' <<<"$block")
aud=$(grep -A1 'audTag:' <<<"$block" | sed -n 's/^ *- *//p')
[[ $team =~ ^[a-z0-9-]+$ ]]
[[ $aud =~ ^[0-9a-f]{64}$ ]]
set -a; . deploy/.env; set +a
login=$(curl -s -o /dev/null -m 20 -w '%{redirect_url}' "https://$LIFEOS_ADMIN_HOST/admin/items")
[[ $login == "https://$team.cloudflareaccess.com/"* ]]
echo "the team name is the team of the login page: yes"
curl -s -m 20 -H @<(printf 'Authorization: Bearer %s' "$CLOUDFLARE_API_TOKEN") "https://api.cloudflare.com/client/v4/accounts/$CLOUDFLARE_ACCOUNT_ID/access/apps" \
    | AUD="$aud" python3 -c 'import json,sys,os; a=next(a for a in json.load(sys.stdin)["result"] if a["name"]=="LifeOS MCP"); ok=a["aud"]==os.environ["AUD"]; print("the tag is the tag of LifeOS MCP:", "yes" if ok else "NO"); sys.exit(0 if ok else 1)'
grep -q '^CF_ACCESS_TEAM=' deploy/.env || printf '\n# Cloudflare Access: the token check of the MCP server (lifeos serve).\nCF_ACCESS_TEAM=%s\nLIFEOS_MCP_AUD=%s\n' "$team" "$aud" >> deploy/.env
grep -c -E '^(CF_ACCESS_TEAM|LIFEOS_MCP_AUD)=.' deploy/.env
```
Expected: the run passes. The output contains `the team name is the team of the login page: yes` and `the tag is the tag of LifeOS MCP: yes`.

The last line is `2`. When the API token of Cloudflare does not work any more, the block stops before it writes. Then compare the tag with the page of the application in the Cloudflare dashboard, and do the last two lines by hand.

- [ ] **Step 5: Let the service token of the Pi call the MCP server**

Gautam approved decision 7. The step makes the policy only when the application has no policy for a service token, so a second run changes nothing.

Run:
```bash
set -e
cd /home/gautam/lifeos-core
set -a; . deploy/.env; set +a
api=https://api.cloudflare.com/client/v4/accounts/$CLOUDFLARE_ACCOUNT_ID/access
cf() { curl -s -m 20 -H @<(printf 'Authorization: Bearer %s' "$CLOUDFLARE_API_TOKEN") "$@"; }
app=$(cf "$api/apps" | python3 -c 'import json,sys; a=next(a for a in json.load(sys.stdin)["result"] if a["name"]=="LifeOS MCP"); print(a["id"] if not any(p.get("decision")=="non_identity" for p in a.get("policies",[])) else "")')
if [ -z "$app" ]; then
    echo "policy made: the application has one already"
else
    cf -X POST -H 'Content-Type: application/json' "$api/apps/$app/policies" \
        -d '{"name":"Devices with a service token","decision":"non_identity","include":[{"any_valid_service_token":{}}],"precedence":2}' \
        | python3 -c 'import json,sys; d=json.load(sys.stdin); print("policy made:", d["success"], [e.get("message") for e in d.get("errors", [])]); sys.exit(0 if d["success"] else 1)'
fi
cf "$api/apps" | python3 -c 'import json,sys; a=next(a for a in json.load(sys.stdin)["result"] if a["name"]=="LifeOS MCP"); print("policies of LifeOS MCP:", sorted(p["decision"] for p in a["policies"]))'
```
Expected: the run passes. The output contains `policy made:` and `policies of LifeOS MCP: ['allow', 'non_identity']`.

When the API refuses, ask Gautam to add the policy in the dashboard: Access, Applications, `LifeOS MCP`, Policies, action "Service Auth", include "Any Access Service Token".

- [ ] **Step 6: Try the new binary on different ports**

The check uses a throwaway database on the server of the tests and an empty wiki below `bin/`. It starts the binary with the real two values of `deploy/.env`, so it proves that the server accepts them. It also proves that the binary does not start without them and that its MCP port refuses a request that has no token.

`make build` replaces `bin/lifeos`, which the unit starts. From this step, a start of the service runs the new binary; Step 4 gave it the values that it needs.

Run:
```bash
set -e
cd /home/gautam/lifeos-core
make build
rm -rf bin/smoke && mkdir -p bin/smoke/files bin/smoke/inbox bin/smoke/wiki-repo/wiki && git init -q bin/smoke/wiki-repo
sed -e 's/^http_addr: .*/http_addr: 127.0.0.1:8290/' -e 's/^mcp_addr: .*/mcp_addr: 127.0.0.1:8191/' \
    -e 's#@127.0.0.1:5432/lifeos?#@127.0.0.1:5433/lifeos_smoke?#' \
    -e "s#^files_dir: .*#files_dir: $PWD/bin/smoke/files#" -e "s#^inbox_dir: .*#inbox_dir: $PWD/bin/smoke/inbox#" \
    -e "s#^wiki_dir: .*#wiki_dir: $PWD/bin/smoke/wiki-repo/wiki#" config/lifeos.yaml > bin/smoke/lifeos.yaml
[ "$(grep -c '@127.0.0.1:5433/lifeos_smoke?' bin/smoke/lifeos.yaml)" = 2 ]
docker exec lifeos-postgres-test-1 psql -U lifeos -d postgres -qc "DROP DATABASE IF EXISTS lifeos_smoke" -c "CREATE DATABASE lifeos_smoke"
set -a; . deploy/.env; set +a
bin/lifeos migrate -config bin/smoke/lifeos.yaml | tail -1
env -u CF_ACCESS_TEAM -u LIFEOS_MCP_AUD bin/lifeos serve -config bin/smoke/lifeos.yaml 2>&1 | tail -1 || true
bin/lifeos serve -config bin/smoke/lifeos.yaml > bin/smoke/serve.log 2>&1 &
server=$!
for i in $(seq 1 40); do curl -s -o /dev/null -m 2 http://127.0.0.1:8290/api/items && break; sleep 0.5; done
first='{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"smoke","version":"1"}}}'
curl -s -m 10 -o /dev/null -w 'no token: %{http_code}\n' -X POST http://127.0.0.1:8191/mcp -H 'Content-Type: application/json' -H 'Accept: application/json, text/event-stream' -d "$first"
curl -s -m 15 -o /dev/null -w 'a text that is not a token: %{http_code}\n' -X POST http://127.0.0.1:8191/mcp -H 'Cf-Access-Jwt-Assertion: not-a-token' -H 'Content-Type: application/json' -H 'Accept: application/json, text/event-stream' -d "$first"
curl -s -m 10 -o /dev/null -w 'the API: %{http_code}\n' http://127.0.0.1:8290/api/items
kill "$server"; wait "$server" 2>/dev/null || true
grep -c 'request refused' bin/smoke/serve.log
docker exec lifeos-postgres-test-1 psql -U lifeos -d postgres -qc "DROP DATABASE lifeos_smoke"
rm -rf bin/smoke
```
Expected: the run passes. The output contains `CF_ACCESS_TEAM: "" is not the team name of Cloudflare Access`, `no token: 403`, `a text that is not a token: 403` and `the API: 200`.

The count of refused requests is `2`. Stop the server with `kill` and its process ID, as the step does: a `pkill -f` with a pattern ends the shell that runs it. When the block stops early, stop the server by hand and drop the database `lifeos_smoke`.

- [ ] **Step 7: Deploy**

Run:
```bash
cd /home/gautam/lifeos-core
deploy/deploy.sh
first='{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"check","version":"1"}}}'
curl -s -m 10 -o /dev/null -w 'the MCP port with no token: %{http_code}\n' -X POST http://127.0.0.1:8100/mcp -H 'Content-Type: application/json' -H 'Accept: application/json, text/event-stream' -d "$first"
systemctl is-active lifeos.service
```
Expected: the run passes. The output contains `lifeos answers on 127.0.0.1:8200`, `status 200 with the service token`, `the MCP port with no token: 403` and `active`.

`LIFEOS_MCP_HOST` is not in `deploy/.env` yet, so the check of the deploy does not look at the MCP hostname. When the server does not start, read `sudo journalctl -u lifeos -n 30`: a line with `CF_ACCESS_TEAM` or `LIFEOS_MCP_AUD` means that Step 4 did not write the value.

- [ ] **Step 8: Point the tunnel to the server and check the hostname**

The rule gets the real MCP port and a limit to the path `/mcp`. The `access` part of the rule stays. `set -e` stops the block before the restart of `cloudflared` when the config is not as expected or not valid: a restart with a bad config drops each hostname, also `ssh.`.

Run:
```bash
set -e -o pipefail
cd /home/gautam/lifeos-core
config=/etc/cloudflared/config.yml
[ "$(sudo grep -c '^    service: http://127.0.0.1:8190$' "$config")" = 1 ]
[ "$(sudo grep -c '127.0.0.1:8100' "$config" || true)" = 0 ]
sudo cp "$config" "$config.bak-$(date +%Y%m%d-%H%M%S)"
sudo sed -i 's|^    service: http://127.0.0.1:8190$|    path: ^/mcp$\n    service: http://127.0.0.1:8100|' "$config"
host=$(sudo sed -n 's/^  - hostname: \(mcp\..*\)$/\1/p' "$config")
sudo cloudflared tunnel --config "$config" ingress validate
sudo cloudflared tunnel --config "$config" ingress rule "https://$host/mcp" | grep 'service: http://127.0.0.1:8100$'
sudo cloudflared tunnel --config "$config" ingress rule "https://$host/other" | grep 'service: http_status:404$'
sudo systemctl restart cloudflared
for i in $(seq 1 30); do [ "$(curl -s -o /dev/null -m 5 -w '%{http_code}' "https://$host/mcp" || true)" = 401 ] && break; sleep 2; done
systemctl is-active cloudflared
grep -q '^LIFEOS_MCP_HOST=' deploy/.env || printf 'LIFEOS_MCP_HOST=%s\n' "$host" >> deploy/.env
deploy/check-access.sh | sed -E 's#https://[^/]+#https://<host>#'
```
Expected: the run passes. The output contains `OK`, `service: http://127.0.0.1:8100`, `service: http_status:404`, `active`, `ok    https://<host>/mcp: Access refuses the request (401)`, `ok    https://<host>/: Access refuses the request (401)` and `ok    https://<host>/mcp: status 200 with the service token`.

No line of the check starts with `OPEN` or `FAIL`.

The line with `status 200` proves the full way with a real token: the tunnel, the key set that the server gets from Cloudflare, the token check and the MCP server. When it shows `status 403`, read `sudo journalctl -u lifeos -n 20`: the reason is in the first line `mcp: request refused` after the start of the server. The reason `the key set did not come` means that the server cannot reach Cloudflare from inside its unit.

When the block stops before the restart, the config can have the change already: put the backup back. When the check prints `OPEN`, put the backup back at once and start `cloudflared` again.

The OAuth pages of the hostname (`/.well-known/...`) are answers of Cloudflare, not of the server: in the spike, only `POST /mcp` reached the server. So the path limit does not stop the login. When the login of Task 8 fails all the same, remove the line `path: ^/mcp$` as the first test.

- [ ] **Step 9: Start the timer of the wiki push**

The run of the unit in this step finds nothing new and makes no connection to GitHub. So one more command proves that a unit of the user `gautam` can reach GitHub with the SSH key.

Run:
```bash
set -e
cd /home/gautam/lifeos-core
sudo systemctl link "$PWD/deploy/systemd/lifeos-wiki-push.service" "$PWD/deploy/systemd/lifeos-wiki-push.timer"
sudo systemctl enable --now lifeos-wiki-push.timer
sudo systemctl start lifeos-wiki-push.service
systemctl show lifeos-wiki-push.service -p Result --value
sudo journalctl -u lifeos-wiki-push.service -n 3 -o cat --no-pager
echo "heads that a unit reads from GitHub: $(sudo systemd-run --quiet --wait --collect --pipe -p User=gautam git --git-dir=/var/backups/lifeos/wiki.git ls-remote --heads origin main | wc -l)"
systemctl list-timers lifeos-wiki-push.timer --no-pager | sed -n 2p | cut -c1-60
```
Expected: the run passes. The output contains `success`, `GitHub has each commit of the wiki` and `heads that a unit reads from GitHub: 1`.

- [ ] **Step 10: Remove what the hostname replaces**

The MCP entry of Claude Code for this folder points to the local port, which answers 403 now. The connector of claude.ai takes its place (Task 8). The source of the spike is not necessary any more.

Run:
```bash
cd /home/gautam/lifeos-core
claude mcp remove lifeos -s local
claude mcp list 2>&1 | grep -c '127.0.0.1:8100' || true
rm -rf .superpowers/spike-mcp
```
Expected: the run passes.

The count is `0`.

- [ ] **Step 11: Read the logs**

Run:
```bash
echo "errors of the server: $(sudo journalctl -u lifeos --since '30 minutes ago' --no-pager | grep -c -E ' ERROR |panic' || true)"
sudo journalctl -u cloudflared --since '15 minutes ago' --no-pager -o cat | grep ' ERR ' | tail -5 | cut -c1-160 || true
echo "files that wait for a commit: $(git -C /home/gautam/lifeos-core status --short | grep -v 'docs/superpowers/plans/' | wc -l)"
```
Expected: the run passes. The output contains `errors of the server: 0` and `files that wait for a commit: 0`.

`cloudflared` writes `ERR` lines when it stops: lines about a closed connection at the time of the restart are correct. A line `mcp: request refused` in the log of the server is correct too: Steps 7 and 8 sent requests that have no token.

---

### Task 8: Confirmation gate

Gautam does this task. The executor gives him the steps and waits for the results.

- [ ] **Step 1: Add the connector**

1. Open <https://claude.ai/customize/connectors> and add a custom connector.
2. Name: `lifeos`. URL: `https://<the MCP hostname>/mcp`. The hostname is the value of `LIFEOS_MCP_HOST` in `deploy/.env`.
3. Connect. Cloudflare Access asks for the email address and sends a code. Type the code.

The connector shows the 19 tools of LifeOS.

- [ ] **Step 2: Three checks from the phone**

In the Claude app on the phone, in a new chat:

1. Say: `journal - first entry through the public hostname`. The agent stores the entry and gives its ID.
2. Say: `what did I write in my journal today?` The agent answers with the entry and cites it.
3. Say: `add one line to the page of the LifeOS project: the MCP hostname is open`. The agent reads the page and adds the line with a citation.

- [ ] **Step 3: One check in Claude Code on the Pi**

Start a new Claude Code session in `/home/gautam/lifeos-core`. Type `/mcp`. The list has the connector `lifeos` of claude.ai, and it has no entry for `127.0.0.1:8100`. Ask: `list my open todos`. The agent calls a tool of the connector.

A session that was open before Task 7 keeps the old entry until it starts again.

- [ ] **Step 4: The wiki gets to GitHub**

Run on the Pi, or wait for the next full hour:
```bash
sudo systemctl start lifeos-wiki-push.service
sudo journalctl -u lifeos-wiki-push.service -n 2 -o cat --no-pager
```
Expected: the run passes. The output contains `pushed`.

The repository `lifeos-wiki` on GitHub shows the commit of Step 2.

- [ ] **Step 5: Close the open points**

1. Revoke the API token of Cloudflare, and empty `CLOUDFLARE_ACCOUNT_ID` and `CLOUDFLARE_API_TOKEN` in `deploy/.env`. No file of `deploy/` reads them.
2. Say `push` to send the commits of this plan to GitHub.

The week of daily use starts when the four checks pass.

---

## After this plan

| Where | Content |
| --- | --- |
| The test week | One week of daily use from the phone through the agent. This is the gate of Phase 3. Each procedure that the agent does wrong becomes a better tool description, or a skill from a draft in `skills/`. |
| Deferred from plan 6 | Nine small findings of its review, such as `wiki_delete` of a page that git does not track, a lint that reports each review page as an orphan, and `log.md` with no rotation. Gautam decides which ones to correct during the test week. |
| Phase 4 | The job runner on systemd timers, the three briefs from `prompts/`, the lint job and the inbox job. A fresh run of `claude -p` must wait until the connector is ready. |
| Not planned | An expiry time for the public keys in memory. A rate limit in the server. A check of the email address in the token: the policy of the Access application decides who gets a token. A merge of a commit that reached GitHub from a different place: `deploy/wiki-push-repair.sh` keeps it on a branch, and the agent writes the change again. A way to change a file outside `wiki/`, such as `.claude/settings.json`, after Task 7. |
