package webapi

import (
	"net/http"
	"strings"

	"github.com/perfectra1n/kritique/internal/configfile"
	"github.com/perfectra1n/kritique/internal/store"
)

// How far the instance is from reviewing, for the Configuration page's
// checklist, and re-reading the repositories a connection's App reaches.

func (s *Server) registerSetup(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/admin/setup", s.admin(s.getSetup))
	mux.HandleFunc("POST /api/v1/admin/connections/{name}/repositories", s.admin(s.registerReached))
}

func (s *Server) getSetup(w http.ResponseWriter, r *http.Request) error {
	writeJSON(w, http.StatusOK, setupStatus(s.current.Get(), s.webURL.String()))
	return nil
}

// setupStatus is f's distance from reviewing, for the dashboard at webURL.
func setupStatus(f *configfile.File, webURL string) SetupStatus {
	base := strings.TrimSuffix(webURL, "/")
	out := SetupStatus{WebURL: base, HooksURL: base + "/hooks/", Connections: []string{}}
	for _, in := range f.Connections {
		out.Connections = append(out.Connections, in.Name)
	}
	if ref := f.Defaults.Review.Model; ref != nil {
		out.ReviewModel = string(*ref)
	}
	out.Embedding = f.Embedding != nil
	return out
}

// registerReached records every repository connection r names reaches on
// an account it serves, so the leader polls them and, with an embedder,
// indexes them before any webhook names them, and disables the ones an
// account's listing no longer has, as the leader's sync does.
func (s *Server) registerReached(w http.ResponseWriter, r *http.Request) error {
	if in, ok := s.current.Get().Connection(r.PathValue("name")); ok && in.Forge.SelfHosted() {
		return s.registerReachedSelfHosted(w, r, in)
	}
	in, app, err := s.connectionApp(r)
	if err != nil {
		return err
	}
	listedAt := s.now()
	accounts, err := app.Reach(r.Context(), in.Accounts)
	if err != nil {
		return errForge(err)
	}
	f := s.current.Get()
	var res RegisterResult
	for _, a := range accounts {
		acct, ok := f.Account(in.Forge, a.Account)
		if !ok || len(a.Repositories) == 0 {
			continue
		}
		repos := make([]store.ReachedRepository, 0, len(a.Repositories))
		for _, x := range a.Repositories {
			repos = append(repos, store.ReachedRepository{
				FullName: x.FullName, DefaultBranch: x.DefaultBranch, Traits: &configfile.RepoTraits{Archived: x.Archived, Fork: x.Fork},
			})
		}
		added, _, err := s.store.RegisterRepositories(r.Context(), acct.ID(), repos, listedAt)
		if err != nil {
			return err
		}
		res.Added += added
	}
	writeJSON(w, http.StatusOK, res)
	return nil
}

// registerReachedSelfHosted is registerReached for a Forgejo or Gitea
// connection, through Reach.
func (s *Server) registerReachedSelfHosted(w http.ResponseWriter, r *http.Request, in *configfile.Connection) error {
	if s.reach == nil {
		return errBadRequest(CodeBadRequest, "this replica cannot list a "+string(in.Forge)+" connection's repositories")
	}
	listedAt := s.now()
	reach, err := s.reach(r.Context(), in)
	if err != nil {
		return errForge(err)
	}
	f := s.current.Get()
	var res RegisterResult
	for _, name := range in.Accounts {
		acct, ok := f.Account(in.Forge, name)
		repos := reach[strings.ToLower(name)]
		if !ok || len(repos) == 0 {
			continue
		}
		added, _, err := s.store.RegisterRepositories(r.Context(), acct.ID(), repos, listedAt)
		if err != nil {
			return err
		}
		res.Added += added
	}
	writeJSON(w, http.StatusOK, res)
	return nil
}
