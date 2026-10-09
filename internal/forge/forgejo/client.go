// Package forgejo implements forge.Client against a Forgejo or Gitea
// instance's REST API (<url>/api/v1) with the standard library's HTTP
// client: neither forge has a Go SDK comparable to go-github, so requests
// and responses are written against the subset of the API kritique needs.
//
// Gitea speaks the REST API, webhooks and OAuth Forgejo does, so one client
// serves both. Where the two have parted, the client asks the forge and
// takes no for an answer: resolving a code conversation and replying in one
// are Gitea's alone (POST /pulls/comments/{id}/resolve, POST
// /pulls/{index}/comments/{id}/replies), and Forgejo replies in a
// conversation by adding a comment to its review instead.
package forgejo

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/perfectra1n/kritique/internal/configfile"
	"github.com/perfectra1n/kritique/internal/forge"
	"github.com/perfectra1n/kritique/internal/webhook"
)

// maxErrorBody bounds how much of an error response an apiError quotes, so
// an HTML error page does not swamp an error message.
const maxErrorBody = 512

// maxDiffBytes bounds a pull request's diff read whole.
const maxDiffBytes = 8 << 20

// pageSize is the limit every listing asks for. Asking explicitly makes a
// short page an unambiguous last page whatever the instance's own default.
const pageSize = 50

// maxPages bounds a listing, so a repository with thousands of pull
// requests, or a pull request with thousands of reviews, is not read to its
// end into memory.
const maxPages = 100

// ErrNotFound wraps every 404 answer, so a caller can branch on a missing
// resource.
var ErrNotFound = errors.New("forgejo: not found")

// errNoContent is a 204 answering a request that expected a body: the
// conversation-comment endpoint's answer for a code comment.
var errNoContent = errors.New("forgejo: no content")

// apiError is a non-2xx answer. Its text names the status as go-github's
// does, "<code> <reason>", which the dashboard reads a forge outage from.
type apiError struct {
	method, path string
	statusCode   int
	body         string
}

func (e *apiError) Error() string {
	return fmt.Sprintf("forgejo: %s %s: %d %s: %s", e.method, e.path, e.statusCode, http.StatusText(e.statusCode), e.body)
}

// Unwrap makes a 404 ErrNotFound.
func (e *apiError) Unwrap() error {
	if e.statusCode == http.StatusNotFound {
		return ErrNotFound
	}
	return nil
}

// status is the HTTP status of the answer err carries, 0 when it carries
// none.
func status(err error) int {
	if e, ok := errors.AsType[*apiError](err); ok {
		return e.statusCode
	}
	return 0
}

// Client is one Forgejo or Gitea instance's API, as one bot account.
type Client struct {
	httpClient *http.Client
	forge      configfile.Forge
	// web is the instance's base URL, api its API's.
	web, api string
	token    string
	// FetchToken, when set, is what GitToken hands a runner in place of
	// the API token, which can write.
	FetchToken string

	mu    sync.Mutex
	login string
	// heads caches the pull request a head commit was last found on, by
	// "owner/repo@sha", so a review's merge base and diff look it up once.
	heads map[string]int
	// noResolve is set once the forge answers that it has no API to
	// resolve a conversation, so it is not asked on every review.
	noResolve atomic.Bool
}

// NewClient builds a Client for the forge at baseURL, an http or https URL
// that may carry a path for an instance served under one, authenticating
// with token. A nil httpClient is http.DefaultClient.
func NewClient(f configfile.Forge, baseURL, token string, httpClient *http.Client) (*Client, error) {
	if !f.SelfHosted() {
		return nil, fmt.Errorf("forgejo: forge %q is not forgejo or gitea", f)
	}
	u, err := url.Parse(baseURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, fmt.Errorf("forgejo: base URL %q is not an http or https URL", baseURL)
	}
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	web := strings.TrimRight(baseURL, "/")
	return &Client{httpClient: httpClient, forge: f, web: web, api: web + "/api/v1", token: token}, nil
}

// do calls the API and decodes a JSON answer into out, when out is not
// nil, or reads a text one into a *string. body, when not nil, is sent as
// JSON.
func (c *Client) do(ctx context.Context, method, path string, body, out any) error {
	var reader io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("forgejo: encode request body: %w", err)
		}
		reader = bytes.NewReader(buf)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.api+path, reader)
	if err != nil {
		return fmt.Errorf("forgejo: build request: %w", err)
	}
	req.Header.Set("Authorization", "token "+c.token)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("forgejo: %s %s: %w", method, path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorBody))
		return &apiError{method: method, path: path, statusCode: resp.StatusCode, body: strings.TrimSpace(string(raw))}
	}
	if out == nil {
		return nil
	}
	if resp.StatusCode == http.StatusNoContent {
		return fmt.Errorf("forgejo: %s %s: %w", method, path, errNoContent)
	}
	if text, ok := out.(*string); ok {
		raw, err := io.ReadAll(io.LimitReader(resp.Body, maxDiffBytes+1))
		if err != nil {
			return fmt.Errorf("forgejo: read %s %s: %w", method, path, err)
		}
		if len(raw) > maxDiffBytes {
			return fmt.Errorf("forgejo: %s %s: answer is over %d bytes", method, path, maxDiffBytes)
		}
		*text = string(raw)
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("forgejo: decode %s %s: %w", method, path, err)
	}
	return nil
}

// list reads every page of the listing at path, which carries a query of
// its own or none, into a slice, up to maxPages of them. stop, when not
// nil, ends the walk at the first item it holds true for, leaving it out.
func list[T any](ctx context.Context, c *Client, path string, stop func(T) bool) ([]T, error) {
	sep := "?"
	if strings.Contains(path, "?") {
		sep = "&"
	}
	var out []T
	for page := 1; page <= maxPages; page++ {
		var batch []T
		if err := c.do(ctx, http.MethodGet, fmt.Sprintf("%s%spage=%d&limit=%d", path, sep, page, pageSize), nil, &batch); err != nil {
			return nil, err
		}
		for _, item := range batch {
			if stop != nil && stop(item) {
				return out, nil
			}
			out = append(out, item)
		}
		if len(batch) < pageSize {
			break
		}
	}
	return out, nil
}

func repoPath(owner, repo string) string {
	return "/repos/" + url.PathEscape(owner) + "/" + url.PathEscape(repo)
}

// issuePath is the path of issue or pull request number, as the issues
// API names both.
func issuePath(owner, repo string, number int) string {
	return fmt.Sprintf("%s/issues/%d", repoPath(owner, repo), number)
}

// pullPath is the path of pull request number.
func pullPath(owner, repo string, number int) string {
	return fmt.Sprintf("%s/pulls/%d", repoPath(owner, repo), number)
}

// escapePath escapes each segment of a repository path.
func escapePath(p string) string {
	segs := strings.Split(p, "/")
	for i, s := range segs {
		segs[i] = url.PathEscape(s)
	}
	return strings.Join(segs, "/")
}

// CloneURL implements forge.Client.
func (c *Client) CloneURL(owner, repo string) string {
	return fmt.Sprintf("%s/%s/%s.git", c.web, url.PathEscape(owner), url.PathEscape(repo))
}

// FileURL implements forge.Client.
func (c *Client) FileURL(owner, repo, sha, path string, line, endLine int) string {
	u := fmt.Sprintf("%s/%s/%s/src/commit/%s/%s#L%d", c.web, url.PathEscape(owner), url.PathEscape(repo), sha, escapePath(path), line)
	if endLine > line {
		u += fmt.Sprintf("-L%d", endLine)
	}
	return u
}

// CommitURL implements forge.Client.
func (c *Client) CommitURL(owner, repo, sha string) string {
	return fmt.Sprintf("%s/%s/%s/commit/%s", c.web, url.PathEscape(owner), url.PathEscape(repo), sha)
}

// ThreadURL implements forge.Client: a code comment of a submitted review
// is anchored on the pull request's conversation by its comment id.
func (c *Client) ThreadURL(owner, repo string, number int, id int64) string {
	return fmt.Sprintf("%s/%s/%s/pulls/%d#issuecomment-%d", c.web, url.PathEscape(owner), url.PathEscape(repo), number, id)
}

// GitToken implements forge.Client: FetchToken when one is configured,
// and otherwise the API token, which can write; Forgejo and Gitea mint no
// token for one repository.
func (c *Client) GitToken(context.Context, string) (string, error) {
	if c.FetchToken != "" {
		return c.FetchToken, nil
	}
	return c.token, nil
}

// ReadGitToken is a credential that can only read the repository:
// FetchToken, which the admin vouches is read-only, and forge.ErrNoReadToken
// without one, since the API token can write.
func (c *Client) ReadGitToken(context.Context, string, string) (string, error) {
	if c.FetchToken == "" {
		return "", forge.ErrNoReadToken
	}
	return c.FetchToken, nil
}

// BranchTip implements forge.Client. An empty branch is the repository's
// default branch.
func (c *Client) BranchTip(ctx context.Context, owner, repo, ref string) (string, string, error) {
	if ref == "" {
		var r repository
		if err := c.do(ctx, http.MethodGet, repoPath(owner, repo), nil, &r); err != nil {
			return "", "", fmt.Errorf("forgejo: repository %s/%s: %w", owner, repo, err)
		}
		ref = r.DefaultBranch
	}
	var b branch
	if err := c.do(ctx, http.MethodGet, repoPath(owner, repo)+"/branches/"+url.PathEscape(ref), nil, &b); err != nil {
		return "", "", fmt.Errorf("forgejo: branch %s of %s/%s: %w", ref, owner, repo, err)
	}
	if b.Commit.ID == "" {
		return "", "", fmt.Errorf("forgejo: branch %s of %s/%s has no commit", ref, owner, repo)
	}
	return b.Commit.ID, ref, nil
}

// CommitSubject implements forge.Client.
func (c *Client) CommitSubject(ctx context.Context, owner, repo, sha string) (string, error) {
	var gc gitCommit
	// stat, verification and files cost the forge work the subject needs
	// none of.
	path := repoPath(owner, repo) + "/git/commits/" + url.PathEscape(sha) + "?stat=false&verification=false&files=false"
	if err := c.do(ctx, http.MethodGet, path, nil, &gc); err != nil {
		return "", fmt.Errorf("forgejo: commit %s of %s/%s: %w", sha, owner, repo, err)
	}
	subject, _, _ := strings.Cut(gc.Commit.Message, "\n")
	return strings.TrimSpace(subject), nil
}

// FileAt implements forge.Client through the raw endpoint.
func (c *Client) FileAt(ctx context.Context, owner, repo, ref, path string) ([]byte, error) {
	var content string
	err := c.do(ctx, http.MethodGet, repoPath(owner, repo)+"/raw/"+escapePath(path)+"?ref="+url.QueryEscape(ref), nil, &content)
	switch {
	case errors.Is(err, ErrNotFound):
		return nil, fmt.Errorf("forgejo: %s of %s/%s at %s: %w", path, owner, repo, ref, fs.ErrNotExist)
	case err != nil:
		return nil, fmt.Errorf("forgejo: %s of %s/%s at %s: %w", path, owner, repo, ref, err)
	case len(content) > forge.MaxFileBytes:
		return nil, fmt.Errorf("forgejo: %s of %s/%s at %s: %w", path, owner, repo, ref, forge.ErrFileTooLarge)
	}
	return []byte(content), nil
}

// BotLogin implements forge.Client, once per process: the account behind a
// token never changes.
func (c *Client) BotLogin(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.login != "" {
		return c.login, nil
	}
	var u user
	if err := c.do(ctx, http.MethodGet, "/user", nil, &u); err != nil {
		return "", fmt.Errorf("forgejo: bot login: %w", err)
	}
	c.login = u.Login
	return c.login, nil
}

// SetStatus implements forge.Client.
func (c *Client) SetStatus(ctx context.Context, owner, repo, sha string, state forge.StatusState, description string) error {
	opts := createStatusOption{State: string(state), Context: forge.StatusContext, Description: forge.StatusDescription(description)}
	if err := c.do(ctx, http.MethodPost, repoPath(owner, repo)+"/statuses/"+url.PathEscape(sha), opts, nil); err != nil {
		return fmt.Errorf("forgejo: status on %s: %w", sha, err)
	}
	return nil
}

// Permission implements forge.Client. Forgejo and Gitea report the
// repository's owner as owner, above their admin, and have no triage or
// maintain level. Only a repository admin may read another login's
// permission, so the bot needs admin access for it.
func (c *Client) Permission(ctx context.Context, owner, repo, login string) (forge.Permission, error) {
	var p collaboratorPermission
	if err := c.do(ctx, http.MethodGet, repoPath(owner, repo)+"/collaborators/"+url.PathEscape(login)+"/permission", nil, &p); err != nil {
		return "", fmt.Errorf("forgejo: permission of %s on %s/%s: %w", login, owner, repo, err)
	}
	level := forge.Permission(p.Permission)
	if p.Permission == "owner" {
		level = forge.PermissionAdmin
	}
	if !level.Valid() {
		return "", fmt.Errorf("forgejo: unrecognized permission %q for %s on %s/%s", p.Permission, login, owner, repo)
	}
	return level, nil
}

// isBot tells a bot by its login: the REST user carries no type, unlike
// GitHub's, so only the "[bot]" suffix webhook.IsBot also reads applies.
func isBot(login string) bool { return webhook.IsBot("", login) }

// sameLogin compares logins as the forge does, ignoring case.
func sameLogin(a, b string) bool { return strings.EqualFold(a, b) }

var _ forge.Client = (*Client)(nil)
