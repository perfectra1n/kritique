package forgejo

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"

	"github.com/perfectra1n/kritique/internal/forge"
)

// RepoLabels lists the names of every label defined on the repository.
func (c *Client) RepoLabels(ctx context.Context, owner, repo string) ([]string, error) {
	labels, err := c.repoLabels(ctx, owner, repo)
	if err != nil {
		return nil, err
	}
	out := make([]string, len(labels))
	for i, l := range labels {
		out[i] = l.Name
	}
	return out, nil
}

func (c *Client) repoLabels(ctx context.Context, owner, repo string) ([]label, error) {
	labels, err := list[label](ctx, c, repoPath(owner, repo)+"/labels", nil)
	if err != nil {
		return nil, fmt.Errorf("forgejo: list labels of %s/%s: %w", owner, repo, err)
	}
	return labels, nil
}

// AddLabels adds labels, by name, to an issue or pull request; one already
// applied is not an error. They are sent by id, which every Forgejo and
// Gitea release takes, where older Gitea releases take no names, so a name
// the repository has no label for adds none of them.
func (c *Client) AddLabels(ctx context.Context, owner, repo string, number int, labels []string) error {
	if len(labels) == 0 {
		return nil
	}
	known, err := c.repoLabels(ctx, owner, repo)
	if err != nil {
		return fmt.Errorf("forgejo: add labels to %s/%s#%d: %w", owner, repo, number, err)
	}
	ids := make([]int64, len(labels))
	for i, name := range labels {
		id, ok := findLabelID(known, name)
		if !ok {
			return fmt.Errorf("forgejo: add labels to %s/%s#%d: the repository has no label %q", owner, repo, number, name)
		}
		ids[i] = id
	}
	if err := c.do(ctx, http.MethodPost, issuePath(owner, repo, number)+"/labels", issueLabelsOption{Labels: ids}, nil); err != nil {
		return fmt.Errorf("forgejo: add labels to %s/%s#%d: %w", owner, repo, number, err)
	}
	return nil
}

// RemoveLabel removes one label, by name, from an issue or pull request; one
// not applied is not an error. The endpoint takes the label's id, so the
// name is looked up among the issue's labels first, and a 404, the label
// removed meanwhile, is not an error either.
func (c *Client) RemoveLabel(ctx context.Context, owner, repo string, number int, name string) error {
	var current []label
	if err := c.do(ctx, http.MethodGet, fmt.Sprintf("%s/issues/%d/labels", repoPath(owner, repo), number), nil, &current); err != nil {
		return fmt.Errorf("forgejo: remove label %q from %s/%s#%d: %w", name, owner, repo, number, err)
	}
	id, ok := findLabelID(current, name)
	if !ok {
		return nil
	}
	err := c.do(ctx, http.MethodDelete, fmt.Sprintf("%s/issues/%d/labels/%d", repoPath(owner, repo), number, id), nil, nil)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return fmt.Errorf("forgejo: remove label %q from %s/%s#%d: %w", name, owner, repo, number, err)
	}
	return nil
}

func findLabelID(labels []label, name string) (int64, bool) {
	for _, l := range labels {
		if l.Name == name {
			return l.ID, true
		}
	}
	return 0, false
}

// SetState opens or closes an issue or pull request.
func (c *Client) SetState(ctx context.Context, owner, repo string, number int, open bool) error {
	state := "closed"
	if open {
		state = "open"
	}
	if err := c.do(ctx, http.MethodPatch, issuePath(owner, repo, number), editIssueOption{State: &state}, nil); err != nil {
		return fmt.Errorf("forgejo: set the state of %s/%s#%d: %w", owner, repo, number, err)
	}
	return nil
}

// AddAssignees adds logins as assignees of an issue or pull request,
// alongside any already assigned. The edit endpoint replaces the whole
// list, so the current assignees are read and merged first.
func (c *Client) AddAssignees(ctx context.Context, owner, repo string, number int, logins []string) error {
	if len(logins) == 0 {
		return nil
	}
	current, err := c.Issue(ctx, owner, repo, number)
	if err != nil {
		return fmt.Errorf("forgejo: add assignees to %s/%s#%d: %w", owner, repo, number, err)
	}
	merged := slices.Clone(current.Assignees)
	for _, login := range logins {
		if !slices.ContainsFunc(merged, func(a string) bool { return sameLogin(a, login) }) {
			merged = append(merged, login)
		}
	}
	if err := c.do(ctx, http.MethodPatch, issuePath(owner, repo, number), editIssueOption{Assignees: merged}, nil); err != nil {
		return fmt.Errorf("forgejo: add assignees to %s/%s#%d: %w", owner, repo, number, err)
	}
	return nil
}

// RequestReviewers requests review from logins on a pull request.
func (c *Client) RequestReviewers(ctx context.Context, owner, repo string, number int, logins []string) error {
	if len(logins) == 0 {
		return nil
	}
	path := fmt.Sprintf("%s/pulls/%d/requested_reviewers", repoPath(owner, repo), number)
	if err := c.do(ctx, http.MethodPost, path, pullReviewRequestOptions{Reviewers: logins}, nil); err != nil {
		return fmt.Errorf("forgejo: request reviewers on %s/%s#%d: %w", owner, repo, number, err)
	}
	return nil
}

// SearchIssues searches the repository's open issues and pull requests by
// the free text of query, returning at most limit: the repository's issue
// listing, whose q parameter is its search, with no type filter so both
// come back.
func (c *Client) SearchIssues(ctx context.Context, owner, repo, query string, limit int) ([]forge.Issue, error) {
	if limit <= 0 {
		return []forge.Issue{}, nil
	}
	var batch []issue
	path := fmt.Sprintf("%s/issues?q=%s&limit=%d&page=1", repoPath(owner, repo), url.QueryEscape(query), limit)
	if err := c.do(ctx, http.MethodGet, path, nil, &batch); err != nil {
		return nil, fmt.Errorf("forgejo: search %q in %s/%s: %w", query, owner, repo, err)
	}
	out := make([]forge.Issue, 0, min(len(batch), limit))
	for _, iss := range batch[:min(len(batch), limit)] {
		out = append(out, issueFrom(iss))
	}
	return out, nil
}
