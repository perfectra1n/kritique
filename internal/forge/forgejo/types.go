package forgejo

import "time"

// The subsets of Forgejo's and Gitea's REST models kritique reads and
// writes, as their swagger documents them (structs in modules/structs).

// user is the REST API's user. It carries no bot flag, unlike GitHub's, so
// a bot is told by its login alone.
type user struct {
	ID       int64  `json:"id"`
	Login    string `json:"login"`
	FullName string `json:"full_name"`
}

// label is a repository's or an issue's label. ID is what the API adds and
// removes a label by on every release.
type label struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Color string `json:"color"`
}

// repository is the subset of the Repository model kritique reads.
type repository struct {
	FullName      string `json:"full_name"`
	DefaultBranch string `json:"default_branch"`
	Archived      bool   `json:"archived"`
	Fork          bool   `json:"fork"`
	Owner         user   `json:"owner"`
}

// branchInfo is a pull request's head or base. Repo is a pointer because a
// deleted fork leaves head.repo null.
type branchInfo struct {
	Ref  string      `json:"ref"`
	SHA  string      `json:"sha"`
	Repo *repository `json:"repo"`
}

// pullRequest is the subset of the PullRequest model kritique reads.
type pullRequest struct {
	Number       int        `json:"number"`
	Title        string     `json:"title"`
	Body         string     `json:"body"`
	User         user       `json:"user"`
	State        string     `json:"state"`
	Merged       bool       `json:"merged"`
	Draft        bool       `json:"draft"`
	HTMLURL      string     `json:"html_url"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
	ClosedAt     *time.Time `json:"closed_at"`
	MergeBase    string     `json:"merge_base"`
	Additions    int        `json:"additions"`
	Deletions    int        `json:"deletions"`
	ChangedFiles int        `json:"changed_files"`
	Head         branchInfo `json:"head"`
	Base         branchInfo `json:"base"`
	Labels       []label    `json:"labels"`
}

// branch is the subset of GET /repos/{owner}/{repo}/branches/{branch}
// kritique reads.
type branch struct {
	Commit struct {
		ID string `json:"id"`
	} `json:"commit"`
}

// gitCommit is the subset of GET /repos/{owner}/{repo}/git/commits/{sha}
// kritique reads.
type gitCommit struct {
	Commit struct {
		Message string `json:"message"`
	} `json:"commit"`
}

// comment is an issue's or a pull request's conversation comment.
type comment struct {
	ID        int64     `json:"id"`
	Body      string    `json:"body"`
	User      user      `json:"user"`
	CreatedAt time.Time `json:"created_at"`
}

// createCommentOption is the body that creates or edits a conversation
// comment.
type createCommentOption struct {
	Body string `json:"body"`
}

// Review states, as PullReview.state and CreatePullReviewOptions.event
// spell them.
const (
	reviewApproved       = "APPROVED"
	reviewComment        = "COMMENT"
	reviewRequestChanges = "REQUEST_CHANGES"
)

// pullReview is one review of a pull request. A dismissed review keeps its
// state and says so in Dismissed.
type pullReview struct {
	ID          int64     `json:"id"`
	User        *user     `json:"user"`
	State       string    `json:"state"`
	CommitID    string    `json:"commit_id"`
	Dismissed   bool      `json:"dismissed"`
	SubmittedAt time.Time `json:"submitted_at"`
}

// pullReviewComment is one inline comment of a review. It names no
// comment it replies to: the replies of a code conversation are the
// comments of the same review on the same line. Position is the line on
// the head side, OriginalPosition the line on the base side; one of them
// is 0. Resolver is the user who resolved the conversation, nil while it
// is open.
type pullReviewComment struct {
	ID               int64     `json:"id"`
	ReviewID         int64     `json:"pull_request_review_id"`
	Body             string    `json:"body"`
	Path             string    `json:"path"`
	Position         int       `json:"position"`
	OriginalPosition int       `json:"original_position"`
	CommitID         string    `json:"commit_id"`
	User             user      `json:"user"`
	Resolver         *user     `json:"resolver"`
	CreatedAt        time.Time `json:"created_at"`
}

// createPullReviewComment is one inline comment of a review being created,
// and the body of POST /pulls/{index}/reviews/{id}/comments, which adds one
// to a review: new_position is a line on the head side, old_position one
// on the base side.
type createPullReviewComment struct {
	Path        string `json:"path"`
	Body        string `json:"body"`
	NewPosition int    `json:"new_position,omitempty"`
	OldPosition int    `json:"old_position,omitempty"`
}

// createPullReviewOptions is the body of POST /pulls/{index}/reviews.
type createPullReviewOptions struct {
	CommitID string                    `json:"commit_id"`
	Event    string                    `json:"event"`
	Body     string                    `json:"body,omitempty"`
	Comments []createPullReviewComment `json:"comments,omitempty"`
}

// replyOptions is the body of Gitea's POST
// /pulls/{index}/comments/{id}/replies.
type replyOptions struct {
	Body string `json:"body"`
}

// dismissOptions is the body of POST /pulls/{index}/reviews/{id}/dismissals.
type dismissOptions struct {
	Message string `json:"message"`
}

// collaboratorPermission is GET
// /repos/{owner}/{repo}/collaborators/{collaborator}/permission.
type collaboratorPermission struct {
	Permission string `json:"permission"`
}

// createStatusOption is the body of POST /repos/{owner}/{repo}/statuses/{sha}.
type createStatusOption struct {
	State       string `json:"state"`
	Context     string `json:"context"`
	Description string `json:"description"`
}

// reaction is a reaction as the API returns one: it carries no id, so one
// is named by its content and its user.
type reaction struct {
	User    *user  `json:"user"`
	Content string `json:"content"`
}

// editReactionOption is the body that adds or removes a reaction.
type editReactionOption struct {
	Content string `json:"content"`
}

// issue is the subset of the Issue model kritique reads. A pull request is
// an issue whose pull_request is set.
type issue struct {
	Number      int              `json:"number"`
	Title       string           `json:"title"`
	Body        string           `json:"body"`
	State       string           `json:"state"`
	User        user             `json:"user"`
	Labels      []label          `json:"labels"`
	Assignees   []user           `json:"assignees"`
	PullRequest *pullRequestMeta `json:"pull_request"`
	HTMLURL     string           `json:"html_url"`
}

// pullRequestMeta is the pull request part of an issue that is one. Draft
// is false on a release that predates it.
type pullRequestMeta struct {
	Draft bool `json:"draft"`
}

// issueLabelsOption is the body of POST
// /repos/{owner}/{repo}/issues/{index}/labels: label ids, which every
// release takes, where older Gitea releases take no names.
type issueLabelsOption struct {
	Labels []int64 `json:"labels"`
}

// editIssueOption is the body of PATCH /repos/{owner}/{repo}/issues/{index}.
// Assignees replaces the whole list, so a caller that adds sends the
// merged set.
type editIssueOption struct {
	State     *string  `json:"state,omitempty"`
	Assignees []string `json:"assignees,omitempty"`
}

// pullReviewRequestOptions is the body of POST
// /repos/{owner}/{repo}/pulls/{index}/requested_reviewers.
type pullReviewRequestOptions struct {
	Reviewers []string `json:"reviewers"`
}
