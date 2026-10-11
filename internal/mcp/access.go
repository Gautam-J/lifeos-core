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
