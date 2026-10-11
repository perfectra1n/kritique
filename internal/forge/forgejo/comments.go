package forgejo

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"slices"
	"strings"

	"github.com/perfectra1n/kritique/internal/configfile"
	"github.com/perfectra1n/kritique/internal/forge"
)

// maxListedComments is how many comments ListConversation and ListInline
// each return, the oldest, as GitHub's client bounds them.
const maxListedComments = 5000

// ErrNoPullRequest is an inline comment asked for by id with no pull
// request in the context: the API finds one only under its pull request's
// reviews (see forge.WithPullRequest).
var ErrNoPullRequest = errors.New("forgejo: an inline comment is found only through its pull request")

// FindComment implements forge.Client.
func (c *Client) FindComment(ctx context.Context, owner, repo string, number int, login, marker string) (int64, error) {
	comments, err := c.ListConversation(ctx, owner, repo, number)
	if err != nil {
		return 0, err
	}
	for _, cm := range comments {
		if sameLogin(cm.Author, login) && strings.Contains(cm.Body, marker) {
			return cm.ID, nil
		}
	}
	return 0, nil
}

// ListConversation implements forge.Client. The forge lists a pull
// request's comments oldest first; code comments are not among them.
func (c *Client) ListConversation(ctx context.Context, owner, repo string, number int) ([]forge.Comment, error) {
	raw, err := list[comment](ctx, c, fmt.Sprintf("%s/issues/%d/comments", repoPath(owner, repo), number), nil)
	if err != nil {
		return nil, fmt.Errorf("forgejo: list comments on %s/%s#%d: %w", owner, repo, number, err)
	}
	out := make([]forge.Comment, 0, min(len(raw), maxListedComments))
	for _, cm := range raw[:min(len(raw), maxListedComments)] {
		out = append(out, conversationComment(cm))
	}
	// Stable: times are whole seconds, and comments of the same second are
	// listed in the order they were made.
	slices.SortStableFunc(out, func(a, b forge.Comment) int { return a.CreatedAt.Compare(b.CreatedAt) })
	return out, nil
}

func conversationComment(cm comment) forge.Comment {
	return forge.Comment{ID: cm.ID, Author: cm.User.Login, AuthorIsBot: isBot(cm.User.Login), Body: cm.Body, CreatedAt: cm.CreatedAt}
}

// CreateComment implements forge.Client.
func (c *Client) CreateComment(ctx context.Context, owner, repo string, number int, body string) (int64, error) {
	var cm comment
	if err := c.do(ctx, http.MethodPost, issuePath(owner, repo, number)+"/comments", createCommentOption{Body: body}, &cm); err != nil {
		return 0, fmt.Errorf("forgejo: comment on %s/%s#%d: %w", owner, repo, number, err)
	}
	return cm.ID, nil
}

// UpdateComment implements forge.Client.
func (c *Client) UpdateComment(ctx context.Context, owner, repo string, id int64, body string) error {
	err := c.do(ctx, http.MethodPatch, fmt.Sprintf("%s/issues/comments/%d", repoPath(owner, repo), id), createCommentOption{Body: body}, nil)
	if errors.Is(err, ErrNotFound) {
		return fmt.Errorf("forgejo: edit comment %d: %w", id, fs.ErrNotExist)
	}
	if err != nil {
		return fmt.Errorf("forgejo: edit comment %d: %w", id, err)
	}
	return nil
}

// GetComment implements forge.Client. A conversation comment is read by
// id. The conversation endpoint answers a code comment with 204, and every
// reply in a code conversation is delivered as a conversation comment, so
// a 204 is looked for inline. An inline comment is found among its pull
// request's, which forge.WithPullRequest must name in ctx.
func (c *Client) GetComment(ctx context.Context, owner, repo string, id int64, inline bool) (forge.Comment, error) {
	if !inline {
		var cm comment
		err := c.do(ctx, http.MethodGet, fmt.Sprintf("%s/issues/comments/%d", repoPath(owner, repo), id), nil, &cm)
		if err == nil {
			return conversationComment(cm), nil
		}
		if !errors.Is(err, errNoContent) {
			return forge.Comment{}, fmt.Errorf("forgejo: comment %d: %w", id, err)
		}
	}
	number, ok := forge.PullRequestFrom(ctx)
	if !ok {
		return forge.Comment{}, fmt.Errorf("forgejo: review comment %d: %w", id, ErrNoPullRequest)
	}
	comments, err := c.inline(ctx, owner, repo, number)
	if err != nil {
		return forge.Comment{}, err
	}
	for _, cm := range comments {
		if cm.ID == id {
			return cm.Comment, nil
		}
	}
	return forge.Comment{}, fmt.Errorf("forgejo: review comment %d of %s/%s#%d: %w", id, owner, repo, number, fs.ErrNotExist)
}

// inlineComment is one code comment with what the forge.Comment leaves
// out and a reply or a resolution needs.
type inlineComment struct {
	forge.Comment
	reviewID int64
	oldLine  int
	resolved bool
}

// ListInline implements forge.Client, from every review of the pull
// request and each review's code comments. The forge records no reply
// link: a code conversation is the comments of one review on one line of
// one file, the first of them its root, so a comment's InReplyTo is that
// root's id. A comment carrying a finding's marker opens a conversation of
// its own whatever precedes it, since a review posts each finding as a
// root. The 👍 and 👎 are read for the bot's roots alone, the findings
// a person reacts to. Whether a comment is outdated the API does not say;
// Outdated is false.
func (c *Client) ListInline(ctx context.Context, owner, repo string, number int) ([]forge.Comment, error) {
	comments, err := c.inline(ctx, owner, repo, number)
	if err != nil {
		return nil, err
	}
	login, err := c.BotLogin(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]forge.Comment, len(comments))
	for i, cm := range comments {
		if cm.InReplyTo == 0 && sameLogin(cm.Author, login) {
			up, down, err := c.reactionCounts(ctx, owner, repo, cm.ID)
			if err != nil {
				return nil, err
			}
			cm.ReactionsUp, cm.ReactionsDown = up, down
		}
		out[i] = cm.Comment
	}
	return out, nil
}

// inline lists the pull request's code comments, oldest first, threaded.
func (c *Client) inline(ctx context.Context, owner, repo string, number int) ([]inlineComment, error) {
	reviews, err := c.reviews(ctx, owner, repo, number)
	if err != nil {
		return nil, err
	}
	var out []inlineComment
	for _, rv := range reviews {
		raw, err := c.reviewComments(ctx, owner, repo, number, rv.ID)
		if err != nil {
			return nil, err
		}
		roots := map[string]int64{}
		for _, cm := range raw {
			key := fmt.Sprintf("%s\x00%d\x00%d", cm.Path, cm.Position, cm.OriginalPosition)
			ic := inlineComment{
				ID: cm.ID, Author: cm.User.Login, AuthorIsBot: isBot(cm.User.Login), Body: cm.Body, CreatedAt: cm.CreatedAt,
				Inline: true, Path: cm.Path, Line: cm.Position,
				reviewID: rv.ID, oldLine: cm.OriginalPosition, resolved: cm.Resolver != nil,
			}
			root, ok := roots[key]
			if ok && !strings.Contains(cm.Body, findingMarker) {
				ic.InReplyTo = root
			} else {
				roots[key] = cm.ID
			}
			out = append(out, ic)
			if len(out) >= maxListedComments {
				break
			}
		}
		if len(out) >= maxListedComments {
			break
		}
	}
	slices.SortStableFunc(out, func(a, b inlineComment) int {
		return cmp.Or(a.CreatedAt.Compare(b.CreatedAt), cmp.Compare(a.ID, b.ID))
	})
	return out, nil
}

// findingMarker starts the hidden marker a finding's comment leads with
// (review.FindingMarker), which this package does not import.
const findingMarker = "<!-- kritique:finding:"

// reviews lists the pull request's reviews, oldest first.
func (c *Client) reviews(ctx context.Context, owner, repo string, number int) ([]pullReview, error) {
	reviews, err := list[pullReview](ctx, c, fmt.Sprintf("%s/pulls/%d/reviews", repoPath(owner, repo), number), nil)
	if err != nil {
		return nil, fmt.Errorf("forgejo: list reviews on %s/%s#%d: %w", owner, repo, number, err)
	}
	slices.SortStableFunc(reviews, func(a, b pullReview) int { return cmp.Compare(a.ID, b.ID) })
	return reviews, nil
}

// reviewComments lists one review's code comments, by id.
func (c *Client) reviewComments(ctx context.Context, owner, repo string, number int, reviewID int64) ([]pullReviewComment, error) {
	var raw []pullReviewComment
	path := fmt.Sprintf("%s/pulls/%d/reviews/%d/comments", repoPath(owner, repo), number, reviewID)
	if err := c.do(ctx, http.MethodGet, path, nil, &raw); err != nil {
		return nil, fmt.Errorf("forgejo: list the comments of review %d on %s/%s#%d: %w", reviewID, owner, repo, number, err)
	}
	slices.SortStableFunc(raw, func(a, b pullReviewComment) int { return cmp.Compare(a.ID, b.ID) })
	return raw, nil
}

// CreateReview implements forge.Client with a COMMENT review, which neither
// approves nor requests changes. A comment sits on one line, its Line: a
// range's other lines are left out, as releases before Forgejo 16 take no
// range. The review's answer does not carry its comments, so they are
// listed after and matched to the ones sent by path and body, in order.
func (c *Client) CreateReview(
	ctx context.Context, owner, repo string, number int, headSHA string, comments []forge.InlineComment,
) ([]int64, error) {
	if len(comments) == 0 {
		return []int64{}, nil
	}
	opts := createPullReviewOptions{CommitID: headSHA, Event: reviewComment, Comments: make([]createPullReviewComment, 0, len(comments))}
	for _, cm := range comments {
		opts.Comments = append(opts.Comments, createPullReviewComment{Path: cm.Path, Body: cm.Body, NewPosition: cm.Line})
	}
	var rv pullReview
	if err := c.do(ctx, http.MethodPost, fmt.Sprintf("%s/pulls/%d/reviews", repoPath(owner, repo), number), opts, &rv); err != nil {
		return nil, fmt.Errorf("forgejo: review %s/%s#%d: %w", owner, repo, number, err)
	}
	ids := make([]int64, len(comments))
	raw, err := c.reviewComments(ctx, owner, repo, number, rv.ID)
	if err != nil {
		return ids, err
	}
	for _, cm := range raw {
		for i, sent := range comments {
			if ids[i] == 0 && cm.Path == sent.Path && cm.Body == sent.Body {
				ids[i] = cm.ID
				break
			}
		}
	}
	return ids, nil
}

// ReplyInline implements forge.Client in the conversation to's thread
// opened: on Gitea through its replies endpoint, and on Forgejo by adding a
// comment on the root's line to the root's review, which is how its own
// pages reply. A forge that takes neither, an older Gitea, gets a review of
// its own on the root's line, which its pages show beside the conversation
// rather than in it.
func (c *Client) ReplyInline(ctx context.Context, owner, repo string, number int, to forge.Comment, body string) (int64, error) {
	ctx = forge.WithPullRequest(ctx, number)
	comments, err := c.inline(ctx, owner, repo, number)
	if err != nil {
		return 0, err
	}
	rootID := cmp.Or(to.InReplyTo, to.ID)
	i := slices.IndexFunc(comments, func(cm inlineComment) bool { return cm.ID == rootID })
	if i < 0 {
		return 0, fmt.Errorf("forgejo: reply to review comment %d of %s/%s#%d: %w", rootID, owner, repo, number, fs.ErrNotExist)
	}
	root := comments[i]
	var reply pullReviewComment
	if c.forge == configfile.ForgeGitea {
		path := fmt.Sprintf("%s/comments/%d/replies", pullPath(owner, repo, number), root.ID)
		err = c.do(ctx, http.MethodPost, path, replyOptions{Body: body}, &reply)
	} else {
		in := createPullReviewComment{Path: root.Path, Body: body, NewPosition: root.Line, OldPosition: root.oldLine}
		err = c.do(ctx, http.MethodPost, fmt.Sprintf("%s/pulls/%d/reviews/%d/comments", repoPath(owner, repo), number, root.reviewID), in, &reply)
	}
	if err == nil {
		return reply.ID, nil
	}
	if s := status(err); s != http.StatusNotFound && s != http.StatusMethodNotAllowed {
		return 0, fmt.Errorf("forgejo: reply to review comment %d of %s/%s#%d: %w", root.ID, owner, repo, number, err)
	}
	head, err := c.readPull(ctx, owner, repo, number)
	if err != nil {
		return 0, fmt.Errorf("forgejo: reply to review comment %d of %s/%s#%d: %w", root.ID, owner, repo, number, err)
	}
	opts := createPullReviewOptions{CommitID: head.Head.SHA, Event: reviewComment, Comments: []createPullReviewComment{
		{Path: root.Path, Body: body, NewPosition: root.Line, OldPosition: root.oldLine},
	}}
	var rv pullReview
	if err := c.do(ctx, http.MethodPost, fmt.Sprintf("%s/pulls/%d/reviews", repoPath(owner, repo), number), opts, &rv); err != nil {
		return 0, fmt.Errorf("forgejo: reply to review comment %d of %s/%s#%d: %w", root.ID, owner, repo, number, err)
	}
	raw, err := c.reviewComments(ctx, owner, repo, number, rv.ID)
	if err != nil || len(raw) == 0 {
		return 0, nil //nolint:nilerr // the reply is posted; only its id is unknown, which the interface allows
	}
	return raw[0].ID, nil
}

// ResolveThread implements forge.Client. Gitea resolves a code
// conversation through POST /pulls/comments/{id}/resolve; Forgejo has no
// API for it, so on Forgejo, and on a Gitea that answers that it has none,
// the thread is left open and false returned: not done, never faked. A
// conversation already resolved, or one someone else has written in when
// onlyOwn, is left as it is.
func (c *Client) ResolveThread(ctx context.Context, owner, repo string, number int, id int64, onlyOwn bool) (bool, error) {
	if c.forge != configfile.ForgeGitea || c.noResolve.Load() {
		return false, nil
	}
	comments, err := c.inline(forge.WithPullRequest(ctx, number), owner, repo, number)
	if err != nil {
		return false, err
	}
	i := slices.IndexFunc(comments, func(cm inlineComment) bool { return cm.ID == id })
	if i < 0 || comments[i].resolved {
		return false, nil
	}
	if onlyOwn {
		login, err := c.BotLogin(ctx)
		if err != nil {
			return false, err
		}
		for _, cm := range comments {
			if (cm.ID == id || cm.InReplyTo == id) && !sameLogin(cm.Author, login) {
				return false, nil
			}
		}
	}
	err = c.do(ctx, http.MethodPost, fmt.Sprintf("%s/pulls/comments/%d/resolve", repoPath(owner, repo), id), nil, nil)
	switch s := status(err); {
	case err == nil:
		return true, nil
	case s == http.StatusNotFound || s == http.StatusMethodNotAllowed:
		// The comment was just listed, so a 404 is the route missing: a
		// Gitea release without the endpoint.
		c.noResolve.Store(true)
		return false, nil
	default:
		return false, fmt.Errorf("forgejo: resolve the thread of review comment %d of %s/%s#%d: %w", id, owner, repo, number, err)
	}
}
