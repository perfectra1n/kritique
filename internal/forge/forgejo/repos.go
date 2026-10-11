package forgejo

import (
	"context"
	"errors"
	"fmt"
	"net/url"
)

// Repository is a repository the bot reaches on an account, as the poller
// registers it.
type Repository struct {
	FullName, DefaultBranch string
	Archived, Fork          bool
}

// Repositories lists the repositories of account, an organization or a
// user, that the bot can see: the organization's listing, or the user's
// when no organization has the name.
func (c *Client) Repositories(ctx context.Context, account string) ([]Repository, error) {
	repos, err := list[repository](ctx, c, "/orgs/"+url.PathEscape(account)+"/repos", nil)
	if errors.Is(err, ErrNotFound) {
		repos, err = list[repository](ctx, c, "/users/"+url.PathEscape(account)+"/repos", nil)
	}
	if err != nil {
		return nil, fmt.Errorf("forgejo: list the repositories of %s: %w", account, err)
	}
	out := make([]Repository, 0, len(repos))
	for _, r := range repos {
		out = append(out, Repository{FullName: r.FullName, DefaultBranch: r.DefaultBranch, Archived: r.Archived, Fork: r.Fork})
	}
	return out, nil
}
