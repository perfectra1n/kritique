package forgejo

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"time"

	"github.com/perfectra1n/kritique/internal/forge"
	"github.com/perfectra1n/kritique/internal/webhook"
)

// maxCachedHeads bounds the head cache; past it the cache starts over.
const maxCachedHeads = 256

// MergeBase implements forge.Client. The API compares no two commits for
// their merge base, but a pull request reports the one it diffs from, so
// this reads the open pull request of base whose head is head: the one
// forge.WithPullRequest names, or else the one the open pull requests'
// listing has. A head no open pull request has, any longer, is an error,
// never a merge base computed for another head.
func (c *Client) MergeBase(ctx context.Context, owner, repo, base, head string) (string, error) {
	pr, err := c.findHead(ctx, owner, repo, base, head)
	if err != nil {
		return "", fmt.Errorf("forgejo: merge base of %s and %s in %s/%s: %w", base, head, owner, repo, err)
	}
	if pr.MergeBase == "" {
		return "", fmt.Errorf("forgejo: pull request %d of %s/%s reports no merge base", pr.Number, owner, repo)
	}
	return pr.MergeBase, nil
}

// PullRequestDiff implements forge.Client. The API diffs a pull request,
// not two commits, so this reads the diff of the pull request whose head
// is head, as MergeBase finds it, when its merge base is base; any other
// diff is an error, never one of other commits.
func (c *Client) PullRequestDiff(ctx context.Context, owner, repo, base, head string) (string, error) {
	pr, err := c.findHead(ctx, owner, repo, "", head)
	if err != nil {
		return "", fmt.Errorf("forgejo: diff %s...%s of %s/%s: %w", base, head, owner, repo, err)
	}
	if pr.MergeBase != base {
		return "", fmt.Errorf("forgejo: diff %s...%s of %s/%s: pull request %d diffs from %s", base, head, owner, repo, pr.Number, pr.MergeBase)
	}
	var diff string
	if err := c.do(ctx, http.MethodGet, fmt.Sprintf("%s/pulls/%d.diff", repoPath(owner, repo), pr.Number), nil, &diff); err != nil {
		return "", fmt.Errorf("forgejo: diff of %s/%s#%d: %w", owner, repo, pr.Number, err)
	}
	// The pull request may have moved on between the two reads.
	if again, err := c.readPull(ctx, owner, repo, pr.Number); err != nil || again.Head.SHA != head || again.MergeBase != base {
		return "", fmt.Errorf("forgejo: diff of %s/%s#%d: the pull request moved on while it was read", owner, repo, pr.Number)
	}
	return diff, nil
}

// findHead finds the open pull request whose head is head, and whose base
// branch is base unless base is "": first the one ctx names or the cache
// remembers, read afresh, then by listing the open pull requests, most
// recently updated first.
func (c *Client) findHead(ctx context.Context, owner, repo, base, head string) (pullRequest, error) {
	matches := func(pr pullRequest) bool {
		return pr.Head.SHA == head && (base == "" || pr.Base.Ref == base) && pr.State == "open"
	}
	key := owner + "/" + repo + "@" + head
	var known []int
	if n, ok := forge.PullRequestFrom(ctx); ok {
		known = append(known, n)
	}
	c.mu.Lock()
	if n, ok := c.heads[key]; ok {
		known = append(known, n)
	}
	c.mu.Unlock()
	for _, n := range known {
		pr, err := c.readPull(ctx, owner, repo, n)
		if err != nil && !errors.Is(err, ErrNotFound) {
			return pullRequest{}, err
		}
		if err == nil && matches(pr) {
			c.rememberHead(key, n)
			return pr, nil
		}
	}
	var found *pullRequest
	_, err := list(ctx, c, repoPath(owner, repo)+"/pulls?state=open&sort=recentupdate", func(pr pullRequest) bool {
		if matches(pr) {
			found = &pr
			return true
		}
		return false
	})
	if err != nil {
		return pullRequest{}, err
	}
	if found == nil {
		return pullRequest{}, fmt.Errorf("no open pull request has head %s", head)
	}
	c.rememberHead(key, found.Number)
	return *found, nil
}

func (c *Client) rememberHead(key string, number int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.heads == nil || len(c.heads) >= maxCachedHeads {
		c.heads = map[string]int{}
	}
	c.heads[key] = number
}

// readPull reads one pull request.
func (c *Client) readPull(ctx context.Context, owner, repo string, number int) (pullRequest, error) {
	var pr pullRequest
	if err := c.do(ctx, http.MethodGet, fmt.Sprintf("%s/pulls/%d", repoPath(owner, repo), number), nil, &pr); err != nil {
		return pullRequest{}, err
	}
	return pr, nil
}

// ListOpenPullRequests implements forge.Client. The forge sorts by update
// time, so the walk stops at the first pull request older than since.
func (c *Client) ListOpenPullRequests(ctx context.Context, owner, repo string, since time.Time) ([]forge.OpenPullRequest, error) {
	prs, err := list(ctx, c, repoPath(owner, repo)+"/pulls?state=open&sort=recentupdate", func(pr pullRequest) bool {
		return pr.UpdatedAt.Before(since)
	})
	if err != nil {
		return nil, fmt.Errorf("forgejo: list open pull requests of %s/%s: %w", owner, repo, err)
	}
	out := make([]forge.OpenPullRequest, 0, len(prs))
	for _, pr := range prs {
		out = append(out, openPullRequest(pr))
	}
	return out, nil
}

// PullRequest implements forge.Client.
func (c *Client) PullRequest(ctx context.Context, owner, repo string, number int) (forge.OpenPullRequest, error) {
	pr, err := c.readPull(ctx, owner, repo, number)
	if errors.Is(err, ErrNotFound) {
		return forge.OpenPullRequest{}, fmt.Errorf("forgejo: pull request %d of %s/%s: %w", number, owner, repo, fs.ErrNotExist)
	}
	if err != nil {
		return forge.OpenPullRequest{}, fmt.Errorf("forgejo: pull request %d of %s/%s: %w", number, owner, repo, err)
	}
	out := openPullRequest(pr)
	out.Additions, out.Deletions, out.ChangedFiles = pr.Additions, pr.Deletions, pr.ChangedFiles
	return out, nil
}

func fullName(r *repository) string {
	if r == nil {
		return ""
	}
	return r.FullName
}

func openPullRequest(pr pullRequest) forge.OpenPullRequest {
	out := forge.OpenPullRequest{
		Number: pr.Number, Title: pr.Title, Author: pr.User.Login, AuthorIsBot: isBot(pr.User.Login),
		State: cmp.Or(pr.State, "open"), Merged: pr.Merged, Draft: pr.Draft,
		// A deleted fork leaves head.repo null, which is not the base
		// repository either, as the webhook parser rules.
		Fork:    pr.Head.Repo == nil || fullName(pr.Head.Repo) != fullName(pr.Base.Repo),
		HeadRef: pr.Head.Ref, HeadSHA: pr.Head.SHA, BaseRef: pr.Base.Ref,
		URL: pr.HTMLURL, Body: pr.Body, CreatedAt: pr.CreatedAt, UpdatedAt: pr.UpdatedAt, ClosedAt: pr.ClosedAt,
	}
	if pr.Base.Repo != nil {
		out.DefaultBranch = pr.Base.Repo.DefaultBranch
	}
	for _, l := range pr.Labels {
		out.Labels = append(out.Labels, webhook.Label{Name: l.Name, Color: l.Color})
	}
	return out
}

// Issue implements forge.Client. The forge files pull requests among the
// issues, so one is returned as such.
func (c *Client) Issue(ctx context.Context, owner, repo string, number int) (forge.Issue, error) {
	var iss issue
	err := c.do(ctx, http.MethodGet, fmt.Sprintf("%s/issues/%d", repoPath(owner, repo), number), nil, &iss)
	if errors.Is(err, ErrNotFound) {
		return forge.Issue{}, fmt.Errorf("forgejo: issue %d of %s/%s: %w", number, owner, repo, fs.ErrNotExist)
	}
	if err != nil {
		return forge.Issue{}, fmt.Errorf("forgejo: issue %d of %s/%s: %w", number, owner, repo, err)
	}
	return issueFrom(iss), nil
}

func issueFrom(iss issue) forge.Issue {
	out := forge.Issue{
		Number: iss.Number, Title: iss.Title, Body: iss.Body, URL: iss.HTMLURL,
		PullRequest: iss.PullRequest != nil, Draft: iss.PullRequest != nil && iss.PullRequest.Draft,
		State: iss.State, Author: iss.User.Login,
	}
	for _, l := range iss.Labels {
		out.Labels = append(out.Labels, l.Name)
	}
	for _, a := range iss.Assignees {
		out.Assignees = append(out.Assignees, a.Login)
	}
	return out
}
