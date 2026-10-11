package auth

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/perfectra1n/kritique/internal/configfile"
)

// forgejoScopes read the profile, its emails and its organizations. An
// instance that grants every OAuth2 token full access ignores them.
var forgejoScopes = []string{"read:user", "read:organization"}

// forgejoMaxPages bounds how many pages of organizations or teams a
// sign-in reads, as githubMaxPages does.
const forgejoMaxPages = 10

// newForgejoProvider signs in through an OAuth2 application of the Forgejo
// or Gitea instance the sign-in's url names. Gitea's OAuth flow and user
// API are Forgejo's.
func newForgejoProvider(s *configfile.SignIn, redirect string, client *http.Client) *forgeProvider {
	web := webBase(s.URL)
	return newForgeProvider(s, web, web+"/api/v1", redirect, forgejoScopes, client, forgejoAPI{})
}

type forgejoAPI struct{}

func (forgejoAPI) identity(ctx context.Context, c apiClient) (Identity, error) {
	var u struct {
		ID        int64  `json:"id"`
		Login     string `json:"login"`
		FullName  string `json:"full_name"`
		Email     string `json:"email"`
		AvatarURL string `json:"avatar_url"`
	}
	if err := c.getOK(ctx, "/user", &u); err != nil {
		return Identity{}, err
	}
	if u.ID == 0 || u.Login == "" {
		return Identity{}, fmt.Errorf("%w: /user has no id or login", ErrForgeAPI)
	}
	id := Identity{Subject: strconv.FormatInt(u.ID, 10), Login: u.Login, Email: u.Email, DisplayName: u.FullName, AvatarURL: u.AvatarURL}
	if err := verifiedEmail(ctx, c, &id); err != nil {
		return Identity{}, err
	}
	return id, nil
}

// member reports whether org is among the user's own organizations, which
// lists private memberships too.
func (forgejoAPI) member(ctx context.Context, c apiClient, org string) (bool, error) {
	orgs, err := forgejoOrgs(ctx, c)
	if err != nil {
		return false, err
	}
	for _, o := range orgs {
		if strings.EqualFold(o, org) {
			return true, nil
		}
	}
	return false, nil
}

// mappingVars reads the user's organizations and teams, a team named
// "org/name", for the role mapping, which sees what a GitHub one does.
func (forgejoAPI) mappingVars(ctx context.Context, c apiClient, id Identity) (map[string]any, error) {
	orgs, err := forgejoOrgs(ctx, c)
	if err != nil {
		return nil, err
	}
	teams := []string{}
	err = forgejoPages(ctx, c, "/user/teams", func(raw json.RawMessage) (int, error) {
		var page []struct {
			Name         string `json:"name"`
			Organization *struct {
				UserName string `json:"username"`
				Name     string `json:"name"`
			} `json:"organization"`
		}
		if err := json.Unmarshal(raw, &page); err != nil {
			return 0, err
		}
		for _, t := range page {
			if t.Organization != nil {
				teams = append(teams, cmp.Or(t.Organization.UserName, t.Organization.Name)+"/"+t.Name)
			}
		}
		return len(page), nil
	})
	if err != nil {
		return nil, err
	}
	return forgeMappingVars(id, orgs, teams), nil
}

// forgejoOrgs lists the logins of the user's organizations.
func forgejoOrgs(ctx context.Context, c apiClient) ([]string, error) {
	orgs := []string{}
	err := forgejoPages(ctx, c, "/user/orgs", func(raw json.RawMessage) (int, error) {
		var page []struct {
			UserName string `json:"username"`
			Name     string `json:"name"`
		}
		if err := json.Unmarshal(raw, &page); err != nil {
			return 0, err
		}
		for _, o := range page {
			orgs = append(orgs, cmp.Or(o.UserName, o.Name))
		}
		return len(page), nil
	})
	return orgs, err
}

// forgejoPageSize is the limit each page asks for, so a short page is the
// last whatever the instance's default.
const forgejoPageSize = 50

// forgejoPages calls fn with each page of the list at path until a page
// comes back short; fn returns how many items the page held.
func forgejoPages(ctx context.Context, c apiClient, path string, fn func(json.RawMessage) (int, error)) error {
	for page := 1; page <= forgejoMaxPages; page++ {
		var raw json.RawMessage
		if err := c.getOK(ctx, fmt.Sprintf("%s?limit=%d&page=%d", path, forgejoPageSize, page), &raw); err != nil {
			return err
		}
		n, err := fn(raw)
		if err != nil {
			return fmt.Errorf("%w: %s: %w", ErrForgeAPI, path, err)
		}
		if n < forgejoPageSize {
			return nil
		}
	}
	return nil
}

// webBase is the scheme and host a sign-in's url names, lowercased, and its
// path without a trailing slash: how kritique has always written a Forgejo
// or Gitea sign-in's origin, which stored identities are keyed by.
func webBase(raw string) string {
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(strings.TrimRight(raw, "/"))
	if err != nil {
		return strings.TrimRight(raw, "/")
	}
	return strings.ToLower(u.Scheme) + "://" + strings.ToLower(u.Host) + strings.TrimRight(u.Path, "/")
}
