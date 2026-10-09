package auth

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"

	"github.com/perfectra1n/kritique/internal/configfile"
)

// forgejoAuth is an auth block with a sign-in of typ, forgejo or gitea, at
// instance.
func forgejoAuth(typ, instance string) string {
	return "auth:\n" + adminPassword + "  " + typ + ":\n    url: " + instance + "\n    clientId: fj\n    clientSecret: { env: TEST_AUTH_SECRET }\n"
}

func TestForgejoProviderURLs(t *testing.T) {
	for _, typ := range []configfile.SignInType{configfile.SignInForgejo, configfile.SignInGitea} {
		t.Run(string(typ), func(t *testing.T) {
			s, ok := testFile(t, forgejoAuth(string(typ), "https://Git.Example.com/sub/")).Auth.SignInByType(typ)
			if !ok {
				t.Fatal("sign-in not configured")
			}
			p, err := buildProvider(t.Context(), s, "https://kritique.example.com/auth/callback/"+string(typ), http.DefaultClient, nil)
			if err != nil {
				t.Fatal(err)
			}
			fp := p.(*forgeProvider)
			if fp.conf.Endpoint.TokenURL != "https://git.example.com/sub/login/oauth/access_token" || fp.apiBase != "https://git.example.com/sub/api/v1" {
				t.Fatalf("token URL %q, API %q", fp.conf.Endpoint.TokenURL, fp.apiBase)
			}
			u, err := url.Parse(p.AuthCodeURL("st", "no", "verifier-verifier-verifier-verifier-verifier"))
			if err != nil {
				t.Fatal(err)
			}
			if got := u.Scheme + "://" + u.Host + u.Path; got != "https://git.example.com/sub/login/oauth/authorize" {
				t.Fatalf("authorize URL %q", got)
			}
			if q := u.Query(); q.Get("scope") != "read:user read:organization" || q.Get("code_challenge_method") != "S256" {
				t.Fatalf("query = %v", q)
			}
			// Identities stored by kritique before are keyed by this origin.
			if got := signInOrigin(s); got != string(typ)+":https://git.example.com/sub" {
				t.Fatalf("origin = %q", got)
			}
		})
	}
}

func TestWebBase(t *testing.T) {
	for in, want := range map[string]string{
		"https://Git.Example.com/":    "https://git.example.com",
		"git.example.com":             "https://git.example.com",
		"HTTP://git.example.com:3000": "http://git.example.com:3000",
		"https://example.com/forge//": "https://example.com/forge",
	} {
		if got := webBase(in); got != want {
			t.Errorf("webBase(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestForgejoAPI(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok" {
			t.Errorf("Authorization = %q", r.Header.Get("Authorization"))
		}
		page := r.URL.Query().Get("page")
		switch {
		case r.URL.Path == "/user":
			_, _ = fmt.Fprint(w, `{"id":7,"login":"alice","full_name":"Alice","email":"a@private","avatar_url":"https://x/a.png"}`)
		case r.URL.Path == "/user/emails":
			_, _ = fmt.Fprint(w, `[{"email":"old@example.com","verified":true},{"email":"a@example.com","primary":true,"verified":true}]`)
		case r.URL.Path == "/user/orgs" && page == "1":
			items := make([]string, 0, forgejoPageSize)
			for i := range forgejoPageSize {
				items = append(items, fmt.Sprintf(`{"username":"org-%d"}`, i))
			}
			_, _ = fmt.Fprint(w, "["+strings.Join(items, ",")+"]")
		case r.URL.Path == "/user/orgs" && page == "2":
			_, _ = fmt.Fprint(w, `[{"name":"Acme"}]`)
		case r.URL.Path == "/user/teams":
			_, _ = fmt.Fprint(w, `[{"name":"Owners","organization":{"username":"Acme"}},{"name":"orphan"}]`)
		default:
			t.Errorf("unexpected request %s", r.URL)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	c := apiClient{base: srv.URL, token: "tok", client: srv.Client()}
	id, err := forgejoAPI{}.identity(t.Context(), c)
	if err != nil || id.Subject != "7" || id.Login != "alice" || id.DisplayName != "Alice" || id.Email != "a@example.com" || !id.EmailVerified {
		t.Fatalf("identity = %+v, %v", id, err)
	}
	for org, want := range map[string]bool{"acme": true, "org-3": true, "globex": false} {
		if got, err := (forgejoAPI{}).member(t.Context(), c, org); err != nil || got != want {
			t.Errorf("member(%s) = %v, %v", org, got, err)
		}
	}
	vars, err := forgejoAPI{}.mappingVars(t.Context(), c, id)
	if err != nil {
		t.Fatal(err)
	}
	orgs := vars["orgs"].([]string)
	if len(orgs) != forgejoPageSize+1 || orgs[forgejoPageSize] != "Acme" || !reflect.DeepEqual(vars["teams"], []string{"Acme/Owners"}) || vars["login"] != "alice" {
		t.Fatalf("vars = %v", vars)
	}
}

func TestForgejoAPIFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusBadGateway) }))
	defer srv.Close()
	c := apiClient{base: srv.URL, token: "tok", client: srv.Client()}
	if _, err := (forgejoAPI{}).member(t.Context(), c, "acme"); !errors.Is(err, ErrForgeAPI) {
		t.Fatalf("member = %v, want ErrForgeAPI", err)
	}
}
