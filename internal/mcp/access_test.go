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
