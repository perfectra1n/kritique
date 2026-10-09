package forgejo

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"

	"github.com/perfectra1n/kritique/internal/forge"
)

// reactionContents are the reactions the bot leaves, and reads, in the
// order their ids are taken from: Forgejo and Gitea give a reaction no id
// of its own, as it is one per user and content, so the id React returns
// stands for its content, which Unreact removes. Both are among the
// reactions every instance allows by default.
var reactionContents = []string{"+1", "-1", "laugh", "hooray", "confused", "heart", "rocket", "eyes"}

// errReactionID is an id React never returned.
var errReactionID = errors.New("forgejo: unknown reaction id")

func reactionID(content string) (int64, error) {
	i := slices.Index(reactionContents, content)
	if i < 0 {
		return 0, fmt.Errorf("forgejo: reaction %q is not one kritique leaves", content)
	}
	return int64(i + 1), nil
}

func reactionContent(id int64) (string, error) {
	if id < 1 || id > int64(len(reactionContents)) {
		return "", fmt.Errorf("%w %d", errReactionID, id)
	}
	return reactionContents[id-1], nil
}

// React implements forge.Client. A code comment's reactions are a
// conversation comment's: one endpoint serves both. Reacting again with
// the same content answers with the reaction already there.
func (c *Client) React(ctx context.Context, owner, repo string, to forge.Comment, content string) (int64, error) {
	id, err := reactionID(content)
	if err != nil {
		return 0, err
	}
	path := fmt.Sprintf("%s/issues/comments/%d/reactions", repoPath(owner, repo), to.ID)
	if err := c.do(ctx, http.MethodPost, path, editReactionOption{Content: content}, nil); err != nil {
		return 0, fmt.Errorf("forgejo: react to comment %d: %w", to.ID, err)
	}
	return id, nil
}

// Unreact implements forge.Client.
func (c *Client) Unreact(ctx context.Context, owner, repo string, from forge.Comment, id int64) error {
	content, err := reactionContent(id)
	if err != nil {
		return err
	}
	path := fmt.Sprintf("%s/issues/comments/%d/reactions", repoPath(owner, repo), from.ID)
	if err := c.do(ctx, http.MethodDelete, path, editReactionOption{Content: content}, nil); err != nil {
		return fmt.Errorf("forgejo: remove reaction %s from comment %d: %w", content, from.ID, err)
	}
	return nil
}

// ReactToPullRequest implements forge.Client. A pull request's own
// reactions are its issue's.
func (c *Client) ReactToPullRequest(ctx context.Context, owner, repo string, number int, content string) (int64, error) {
	id, err := reactionID(content)
	if err != nil {
		return 0, err
	}
	path := fmt.Sprintf("%s/issues/%d/reactions", repoPath(owner, repo), number)
	if err := c.do(ctx, http.MethodPost, path, editReactionOption{Content: content}, nil); err != nil {
		return 0, fmt.Errorf("forgejo: react to #%d: %w", number, err)
	}
	return id, nil
}

// UnreactToPullRequest implements forge.Client.
func (c *Client) UnreactToPullRequest(ctx context.Context, owner, repo string, number int, id int64) error {
	content, err := reactionContent(id)
	if err != nil {
		return err
	}
	path := fmt.Sprintf("%s/issues/%d/reactions", repoPath(owner, repo), number)
	if err := c.do(ctx, http.MethodDelete, path, editReactionOption{Content: content}, nil); err != nil {
		return fmt.Errorf("forgejo: remove reaction %s from #%d: %w", content, number, err)
	}
	return nil
}

// reactionCounts counts the 👍 and 👎 on a comment.
func (c *Client) reactionCounts(ctx context.Context, owner, repo string, id int64) (up, down int, err error) {
	var reactions []reaction
	path := fmt.Sprintf("%s/issues/comments/%d/reactions", repoPath(owner, repo), id)
	if err := c.do(ctx, http.MethodGet, path, nil, &reactions); err != nil {
		if errors.Is(err, errNoContent) {
			return 0, 0, nil
		}
		return 0, 0, fmt.Errorf("forgejo: reactions of comment %d: %w", id, err)
	}
	for _, r := range reactions {
		switch r.Content {
		case "+1":
			up++
		case "-1":
			down++
		}
	}
	return up, down, nil
}
