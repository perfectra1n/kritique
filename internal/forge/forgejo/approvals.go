package forgejo

import (
	"context"
	"fmt"
	"net/http"
	"slices"
)

// Approve implements forge.Client. The forge keeps every review a user
// submits, so the bot's approval of the head, when one stands, is left as
// it is rather than stacked.
func (c *Client) Approve(ctx context.Context, owner, repo string, number int, headSHA, body string) (bool, error) {
	approvals, err := c.approvals(ctx, owner, repo, number)
	if err != nil {
		return false, err
	}
	if slices.ContainsFunc(approvals, func(r pullReview) bool { return r.CommitID == headSHA }) {
		return false, nil
	}
	opts := createPullReviewOptions{CommitID: headSHA, Event: reviewApproved, Body: body}
	if err := c.do(ctx, http.MethodPost, fmt.Sprintf("%s/pulls/%d/reviews", repoPath(owner, repo), number), opts, nil); err != nil {
		return false, fmt.Errorf("forgejo: approve %s/%s#%d: %w", owner, repo, number, err)
	}
	return true, nil
}

// DismissApprovals implements forge.Client. Only a repository admin may
// dismiss a review; a bot that is not one has its own approval deleted
// instead, which a reviewer may always do, so that it stands no longer
// either way. The message then goes nowhere but the summary.
func (c *Client) DismissApprovals(ctx context.Context, owner, repo string, number int, message string) (int, error) {
	approvals, err := c.approvals(ctx, owner, repo, number)
	if err != nil {
		return 0, err
	}
	for i, r := range approvals {
		review := fmt.Sprintf("%s/pulls/%d/reviews/%d", repoPath(owner, repo), number, r.ID)
		err := c.do(ctx, http.MethodPost, review+"/dismissals", dismissOptions{Message: message}, nil)
		if status(err) == http.StatusForbidden {
			err = c.do(ctx, http.MethodDelete, review, nil, nil)
		}
		if err != nil {
			return i, fmt.Errorf("forgejo: dismiss review %d on %s/%s#%d: %w", r.ID, owner, repo, number, err)
		}
	}
	return len(approvals), nil
}

// ChangesRequested implements forge.Client. Where a reviewer stands is
// their latest review that approved or requested changes, unless it was
// dismissed since; a comment changes nothing.
func (c *Client) ChangesRequested(ctx context.Context, owner, repo string, number int) (bool, error) {
	login, err := c.BotLogin(ctx)
	if err != nil {
		return false, err
	}
	reviews, err := c.reviews(ctx, owner, repo, number)
	if err != nil {
		return false, err
	}
	stands := map[string]string{}
	for _, r := range reviews {
		if r.User == nil || sameLogin(r.User.Login, login) {
			continue
		}
		switch {
		case r.Dismissed && (r.State == reviewApproved || r.State == reviewRequestChanges):
			stands[r.User.Login] = "DISMISSED"
		case r.State == reviewApproved || r.State == reviewRequestChanges:
			stands[r.User.Login] = r.State
		}
	}
	for _, state := range stands {
		if state == reviewRequestChanges {
			return true, nil
		}
	}
	return false, nil
}

// approvals lists the bot's reviews of the pull request that approve it
// and stand.
func (c *Client) approvals(ctx context.Context, owner, repo string, number int) ([]pullReview, error) {
	login, err := c.BotLogin(ctx)
	if err != nil {
		return nil, err
	}
	reviews, err := c.reviews(ctx, owner, repo, number)
	if err != nil {
		return nil, err
	}
	return slices.DeleteFunc(reviews, func(r pullReview) bool {
		return r.User == nil || !sameLogin(r.User.Login, login) || r.State != reviewApproved || r.Dismissed
	}), nil
}
