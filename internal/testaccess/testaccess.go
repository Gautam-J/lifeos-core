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
