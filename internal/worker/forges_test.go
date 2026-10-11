package worker

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/perfectra1n/kritique/internal/configfile"
	"github.com/perfectra1n/kritique/internal/forge"
)

// appConnection is a connection of an App with clientID, its private
// key and webhook secret read from TEST_PRIVATE_KEY and TEST_WEBHOOK_SECRET.
func appConnection(t *testing.T, clientID string) *configfile.Connection {
	t.Helper()
	file, err := configfile.Parse([]byte(`
apps:
  acme-bot:
    accounts: [acme]
    clientId: ` + clientID + `
    privateKey: { env: TEST_PRIVATE_KEY }
    webhookSecret: { env: TEST_WEBHOOK_SECRET }
`))
	if err != nil {
		t.Fatal(err)
	}
	in, _ := file.Connection("acme-bot")
	return in
}

func TestAppsRefuseAnotherForge(t *testing.T) {
	apps := &Apps{}
	other := &configfile.Connection{Name: "x", Forge: "gitlab"}
	if _, err := apps.Build(t.Context(), other, "acme/widgets"); err == nil {
		t.Fatal("Build built a client for a forge kritique does not support")
	}
	if _, err := apps.Reach(t.Context(), other); err == nil {
		t.Fatal("Reach listed a forge kritique does not support")
	}
}

// TestAppsBuildOnePerConnection: a connection's clients and listings
// share one App, and with it its installation tokens; another connection
// gets its own.
func TestAppsBuildOnePerConnection(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("TEST_PRIVATE_KEY", string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})))
	t.Setenv("TEST_WEBHOOK_SECRET", "s")
	acme, globex := appConnection(t, "Iv1.acme"), appConnection(t, "Iv1.globex")
	globex.Name = "globex-bot"
	apps := &Apps{}
	first, err := apps.app(acme)
	if err != nil {
		t.Fatal(err)
	}
	again, err := apps.app(acme)
	if err != nil {
		t.Fatal(err)
	}
	other, err := apps.app(globex)
	if err != nil {
		t.Fatal(err)
	}
	if first != again || first == other || len(apps.apps) != 2 {
		t.Fatalf("apps = %d, want one per connection, the same on every use", len(apps.apps))
	}
}

func TestForgeCacheBuildsOnce(t *testing.T) {
	t.Setenv("TEST_WEBHOOK_SECRET", "s")
	t.Setenv("TEST_PRIVATE_KEY", "pem")
	builds := 0
	cache := &ForgeCache{Build: func(context.Context, *configfile.Connection, string) (forge.Client, error) {
		builds++
		return nil, nil
	}}
	in := appConnection(t, "Iv1.a")
	for range 3 {
		if _, err := cache.For(t.Context(), in, "acme/widgets"); err != nil {
			t.Fatal(err)
		}
	}
	if builds != 1 || len(cache.clients) != 1 {
		t.Fatalf("builds = %d, clients = %d; want one per connection and owner", builds, len(cache.clients))
	}
}

// TestForgeCacheBuildsPerOwner: a GitHub App has a connection, and a
// token, per account, so repositories of different owners get their own
// client and repositories of one owner share one.
func TestForgeCacheBuildsPerOwner(t *testing.T) {
	t.Setenv("TEST_PRIVATE_KEY", "pem")
	t.Setenv("TEST_WEBHOOK_SECRET", "s")
	in := appConnection(t, "Iv1.a")
	var built []string
	cache := &ForgeCache{Build: func(_ context.Context, _ *configfile.Connection, repo string) (forge.Client, error) {
		built = append(built, repo)
		return nil, nil
	}}
	for _, repo := range []string{"acme/widgets", "acme/gadgets", "Other/tools", "other/more"} {
		if _, err := cache.For(t.Context(), in, repo); err != nil {
			t.Fatal(err)
		}
	}
	if want := []string{"acme/widgets", "Other/tools"}; !slices.Equal(built, want) {
		t.Fatalf("built for %q, want %q", built, want)
	}
}

// TestForgeCacheBuildsOutsideItsLock: a build that hangs on the forge
// holds up neither another connection's client nor a caller whose context
// ends meanwhile, and the callers of its own key share the one build.
func TestForgeCacheBuildsOutsideItsLock(t *testing.T) {
	t.Setenv("TEST_PRIVATE_KEY", "pem")
	t.Setenv("TEST_WEBHOOK_SECRET", "s")
	slow, fast := appConnection(t, "Iv1.slow"), appConnection(t, "Iv1.fast")
	fast.Name = "fast-bot"
	release := make(chan struct{})
	var mu sync.Mutex
	builds := map[string]int{}
	cache := &ForgeCache{Build: func(_ context.Context, in *configfile.Connection, _ string) (forge.Client, error) {
		mu.Lock()
		builds[in.Name]++
		mu.Unlock()
		if in == slow {
			<-release
		}
		return nil, nil
	}}
	// Two callers of the slow connection share one build.
	started := make(chan struct{}, 2)
	for range 2 {
		go func() {
			started <- struct{}{}
			_, _ = cache.For(t.Context(), slow, "acme/widgets")
		}()
	}
	<-started
	<-started
	done := make(chan error, 1)
	go func() {
		_, err := cache.For(t.Context(), fast, "acme/widgets")
		done <- err
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("another connection's client waited on the slow build")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()
	if _, err := cache.For(ctx, slow, "acme/gadgets"); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("a caller whose context ended got %v, want its deadline", err)
	}
	close(release)
	if _, err := cache.For(t.Context(), slow, "acme/widgets"); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if builds[slow.Name] != 1 || builds[fast.Name] != 1 {
		t.Fatalf("builds = %v, want one per connection", builds)
	}
}

// TestForgeCacheBuildOutlivesItsFirstCaller: the caller that started a
// build ending does not fail the build for the callers sharing it.
func TestForgeCacheBuildOutlivesItsFirstCaller(t *testing.T) {
	t.Setenv("TEST_PRIVATE_KEY", "pem")
	t.Setenv("TEST_WEBHOOK_SECRET", "s")
	in := appConnection(t, "Iv1.app")
	building, release := make(chan struct{}), make(chan struct{})
	cache := &ForgeCache{Build: func(ctx context.Context, _ *configfile.Connection, _ string) (forge.Client, error) {
		close(building)
		<-release
		return nil, ctx.Err()
	}}
	first, cancel := context.WithCancel(t.Context())
	firstDone := make(chan error, 1)
	go func() {
		_, err := cache.For(first, in, "acme/widgets")
		firstDone <- err
	}()
	<-building
	second := make(chan error, 1)
	go func() {
		_, err := cache.For(t.Context(), in, "acme/widgets")
		second <- err
	}()
	cancel()
	if err := <-firstDone; !errors.Is(err, context.Canceled) {
		t.Fatalf("the cancelled caller got %v, want its cancellation", err)
	}
	// The second caller may join the build or find it done; either way the
	// build must not have seen the first caller's cancellation.
	close(release)
	if err := <-second; err != nil {
		t.Fatalf("a caller sharing the build got %v", err)
	}
}

// TestAppsForgejo: a Forgejo connection gets one client for every owner it
// serves, fetching with its gitToken, and reaches each account's
// repositories, an organization's or a user's.
func TestAppsForgejo(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/orgs/acme/repos":
			_, _ = w.Write([]byte(`[{"full_name":"acme/widgets","default_branch":"main","archived":true}]`))
		case "/api/v1/orgs/Me/repos":
			w.WriteHeader(http.StatusNotFound)
		case "/api/v1/users/Me/repos":
			_, _ = w.Write([]byte(`[{"full_name":"Me/dots","default_branch":"trunk","fork":true}]`))
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	t.Setenv("TEST_WEBHOOK_SECRET", "s")
	t.Setenv("TEST_TOKEN", "write")
	t.Setenv("TEST_GIT_TOKEN", "read")
	file, err := configfile.Parse([]byte(`
apps:
  home:
    forge: forgejo
    url: ` + srv.URL + `
    accounts: [acme, Me]
    token: { env: TEST_TOKEN }
    gitToken: { env: TEST_GIT_TOKEN }
    webhookSecret: { env: TEST_WEBHOOK_SECRET }
`))
	if err != nil {
		t.Fatal(err)
	}
	in, _ := file.Connection("home")
	apps := &Apps{}
	a, err := apps.Build(t.Context(), in, "acme/widgets")
	if err != nil {
		t.Fatal(err)
	}
	b, err := apps.Build(t.Context(), in, "Me/dots")
	if err != nil || a != b {
		t.Fatalf("a second owner's client = %v, %v; want the connection's one", b, err)
	}
	if tok, err := a.GitToken(t.Context(), "widgets"); err != nil || tok != "read" {
		t.Fatalf("GitToken = %q, %v; want the gitToken", tok, err)
	}
	if got := a.CloneURL("acme", "widgets"); got != srv.URL+"/acme/widgets.git" {
		t.Fatalf("CloneURL = %s", got)
	}
	reach, err := apps.Reach(t.Context(), in)
	if err != nil {
		t.Fatal(err)
	}
	if r := reach["acme"]; len(r) != 1 || r[0].FullName != "acme/widgets" || !r[0].Traits.Archived {
		t.Fatalf("acme reaches %+v", r)
	}
	if r := reach["me"]; len(r) != 1 || r[0].DefaultBranch != "trunk" || !r[0].Traits.Fork {
		t.Fatalf("me reaches %+v", r)
	}
}
