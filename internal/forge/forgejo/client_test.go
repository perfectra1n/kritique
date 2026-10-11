package forgejo

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/perfectra1n/kritique/internal/configfile"
	"github.com/perfectra1n/kritique/internal/forge"
)

const testToken = "tok"

// fakeAPI is a Forgejo API under /sub, the path an instance served under
// one has, recording each request it answers.
type fakeAPI struct {
	t   *testing.T
	mux *http.ServeMux

	mu       sync.Mutex
	requests []string
	bodies   map[string]string
}

func newFake(t *testing.T, f configfile.Forge) (*fakeAPI, *Client) {
	t.Helper()
	fake := &fakeAPI{t: t, mux: http.NewServeMux(), bodies: map[string]string{}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "token "+testToken {
			t.Errorf("Authorization = %q", got)
		}
		body, _ := io.ReadAll(r.Body)
		r.Body = io.NopCloser(bytes.NewReader(body))
		key := r.Method + " " + strings.TrimPrefix(r.URL.Path, "/sub/api/v1")
		fake.mu.Lock()
		fake.requests = append(fake.requests, key)
		if len(body) > 0 {
			fake.bodies[key] = string(body)
		}
		fake.mu.Unlock()
		r.URL.Path = strings.TrimPrefix(r.URL.Path, "/sub/api/v1")
		fake.mux.ServeHTTP(w, r)
	}))
	t.Cleanup(srv.Close)
	c, err := NewClient(f, srv.URL+"/sub/", testToken, srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	return fake, c
}

func (f *fakeAPI) reply(pattern string, code int, body string) {
	f.mux.HandleFunc(pattern, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		_, _ = w.Write([]byte(body))
	})
}

// sent is every request answered so far.
func (f *fakeAPI) sent() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.requests)
}

func (f *fakeAPI) asked(key string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Contains(f.requests, key)
}

func (f *fakeAPI) body(key string) map[string]any {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out map[string]any
	if err := json.Unmarshal([]byte(f.bodies[key]), &out); err != nil {
		f.t.Fatalf("body of %s = %q: %v", key, f.bodies[key], err)
	}
	return out
}

func TestNewClient(t *testing.T) {
	for _, tt := range []struct {
		forge configfile.Forge
		url   string
		ok    bool
	}{
		{configfile.ForgeForgejo, "https://git.example.com", true},
		{configfile.ForgeGitea, "http://gitea.internal:3000/sub/", true},
		{configfile.ForgeGitHub, "https://github.com", false},
		{configfile.ForgeForgejo, "git.example.com", false},
		{configfile.ForgeForgejo, "ssh://git.example.com", false},
	} {
		_, err := NewClient(tt.forge, tt.url, "t", nil)
		if (err == nil) != tt.ok {
			t.Errorf("NewClient(%s, %s) = %v", tt.forge, tt.url, err)
		}
	}
}

func TestURLs(t *testing.T) {
	c, err := NewClient(configfile.ForgeForgejo, "https://git.example.com/", "t", nil)
	if err != nil {
		t.Fatal(err)
	}
	for got, want := range map[string]string{
		c.CloneURL("o", "r"):                           "https://git.example.com/o/r.git",
		c.FileURL("o", "r", "abc", "dir/a b.go", 3, 0): "https://git.example.com/o/r/src/commit/abc/dir/a%20b.go#L3",
		c.FileURL("o", "r", "abc", "a.go", 3, 9):       "https://git.example.com/o/r/src/commit/abc/a.go#L3-L9",
		c.CommitURL("o", "r", "abc"):                   "https://git.example.com/o/r/commit/abc",
		c.ThreadURL("o", "r", 4, 77):                   "https://git.example.com/o/r/pulls/4#issuecomment-77",
	} {
		if got != want {
			t.Errorf("got %s, want %s", got, want)
		}
	}
}

func TestGitTokens(t *testing.T) {
	c, _ := NewClient(configfile.ForgeForgejo, "https://git.example.com", "write", nil)
	if tok, _ := c.GitToken(t.Context(), "r"); tok != "write" {
		t.Fatalf("GitToken without a fetch token = %q, want the API token", tok)
	}
	if _, err := c.ReadGitToken(t.Context(), "o", "r"); !errors.Is(err, forge.ErrNoReadToken) {
		t.Fatalf("ReadGitToken without a fetch token = %v, want forge.ErrNoReadToken", err)
	}
	c.FetchToken = "read"
	if tok, _ := c.GitToken(t.Context(), "r"); tok != "read" {
		t.Fatalf("GitToken = %q, want the fetch token", tok)
	}
	if tok, err := c.ReadGitToken(t.Context(), "o", "r"); err != nil || tok != "read" {
		t.Fatalf("ReadGitToken = %q, %v", tok, err)
	}
}

func TestErrors(t *testing.T) {
	f, c := newFake(t, configfile.ForgeForgejo)
	f.reply("GET /repos/o/r/branches/gone", 404, `{"message":"not found"}`)
	f.reply("GET /repos/o/r/branches/down", 502, `<html>bad gateway</html>`)
	_, _, err := c.BranchTip(t.Context(), "o", "r", "gone")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("404 = %v, want ErrNotFound", err)
	}
	_, _, err = c.BranchTip(t.Context(), "o", "r", "down")
	// The dashboard reads an outage from the prefix and the status.
	if err == nil || !strings.HasPrefix(err.Error(), "forgejo: ") || !strings.Contains(err.Error(), ": 502 Bad Gateway: <html>") {
		t.Fatalf("502 = %v", err)
	}
}

func TestBranchTipAndCommitSubject(t *testing.T) {
	f, c := newFake(t, configfile.ForgeForgejo)
	f.reply("GET /repos/o/r", 200, `{"full_name":"o/r","default_branch":"trunk"}`)
	f.reply("GET /repos/o/r/branches/trunk", 200, `{"commit":{"id":"abc123"}}`)
	f.reply("GET /repos/o/r/git/commits/abc123", 200, `{"commit":{"message":"  fix: a thing \n\nbody"}}`)
	sha, branch, err := c.BranchTip(t.Context(), "o", "r", "")
	if err != nil || sha != "abc123" || branch != "trunk" {
		t.Fatalf("BranchTip = %s, %s, %v", sha, branch, err)
	}
	if s, err := c.CommitSubject(t.Context(), "o", "r", "abc123"); err != nil || s != "fix: a thing" {
		t.Fatalf("CommitSubject = %q, %v", s, err)
	}
}

func TestFileAt(t *testing.T) {
	f, c := newFake(t, configfile.ForgeForgejo)
	f.mux.HandleFunc("GET /repos/o/r/raw/dir/a.yaml", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("ref") != "base1" {
			t.Errorf("ref = %q", r.URL.Query().Get("ref"))
		}
		_, _ = w.Write([]byte("key: v\n"))
	})
	f.reply("GET /repos/o/r/raw/gone", 404, `{}`)
	f.mux.HandleFunc("GET /repos/o/r/raw/big", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(bytes.Repeat([]byte("x"), forge.MaxFileBytes+1))
	})
	if b, err := c.FileAt(t.Context(), "o", "r", "base1", "dir/a.yaml"); err != nil || string(b) != "key: v\n" {
		t.Fatalf("FileAt = %q, %v", b, err)
	}
	if _, err := c.FileAt(t.Context(), "o", "r", "base1", "gone"); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("FileAt of a missing file = %v", err)
	}
	if _, err := c.FileAt(t.Context(), "o", "r", "base1", "big"); !errors.Is(err, forge.ErrFileTooLarge) {
		t.Fatalf("FileAt of a big file = %v", err)
	}
}

func TestSetStatusAndPermission(t *testing.T) {
	f, c := newFake(t, configfile.ForgeGitea)
	f.reply("POST /repos/o/r/statuses/abc", 201, `{}`)
	f.reply("GET /repos/o/r/collaborators/boss/permission", 200, `{"permission":"owner"}`)
	f.reply("GET /repos/o/r/collaborators/dev/permission", 200, `{"permission":"write"}`)
	f.reply("GET /repos/o/r/collaborators/odd/permission", 200, `{"permission":"superuser"}`)
	if err := c.SetStatus(t.Context(), "o", "r", "abc", forge.StatusPending, strings.Repeat("é", 200)); err != nil {
		t.Fatal(err)
	}
	b := f.body("POST /repos/o/r/statuses/abc")
	if b["context"] != "kritique/review" || b["state"] != "pending" || len([]rune(b["description"].(string))) != forge.MaxStatusDescription {
		t.Fatalf("status = %v", b)
	}
	for login, want := range map[string]forge.Permission{"boss": forge.PermissionAdmin, "dev": forge.PermissionWrite} {
		if p, err := c.Permission(t.Context(), "o", "r", login); err != nil || p != want {
			t.Errorf("Permission(%s) = %s, %v; want %s", login, p, err, want)
		}
	}
	if _, err := c.Permission(t.Context(), "o", "r", "odd"); err == nil {
		t.Fatal("an unknown level must be an error")
	}
}

func TestIssue(t *testing.T) {
	f, c := newFake(t, configfile.ForgeForgejo)
	f.reply("GET /repos/o/r/issues/12", 200, `{"number":12,"title":"Widgets leak","body":"Steps.","state":"open",
		"user":{"login":"u"},"labels":[{"id":1,"name":"bug"}],"assignees":[{"login":"a"}],"html_url":"https://git.example.com/o/r/issues/12",
		"pull_request":null}`)
	f.reply("GET /repos/o/r/issues/13", 200, `{"number":13,"state":"open","pull_request":{"draft":true}}`)
	f.reply("GET /repos/o/r/issues/14", 404, `{}`)
	got, err := c.Issue(t.Context(), "o", "r", 12)
	want := forge.Issue{
		Number: 12, Title: "Widgets leak", Body: "Steps.", URL: "https://git.example.com/o/r/issues/12",
		State: "open", Author: "u", Labels: []string{"bug"}, Assignees: []string{"a"},
	}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("Issue = %+v, %v", got, err)
	}
	if got, err := c.Issue(t.Context(), "o", "r", 13); err != nil || !got.PullRequest || !got.Draft {
		t.Fatalf("Issue of a draft pull request = %+v, %v", got, err)
	}
	if _, err := c.Issue(t.Context(), "o", "r", 14); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("Issue of an unknown number = %v", err)
	}
}

func TestRepositories(t *testing.T) {
	f, c := newFake(t, configfile.ForgeForgejo)
	f.reply("GET /orgs/acme/repos", 200, `[{"full_name":"acme/a","default_branch":"main","archived":true},{"full_name":"acme/b","fork":true}]`)
	f.reply("GET /orgs/me/repos", 404, `{}`)
	f.reply("GET /users/me/repos", 200, `[{"full_name":"me/c","default_branch":"dev"}]`)
	got, err := c.Repositories(t.Context(), "acme")
	if err != nil || !reflect.DeepEqual(got, []Repository{{FullName: "acme/a", DefaultBranch: "main", Archived: true}, {FullName: "acme/b", Fork: true}}) {
		t.Fatalf("org repositories = %+v, %v", got, err)
	}
	got, err = c.Repositories(t.Context(), "me")
	if err != nil || !reflect.DeepEqual(got, []Repository{{FullName: "me/c", DefaultBranch: "dev"}}) {
		t.Fatalf("user repositories = %+v, %v", got, err)
	}
}
