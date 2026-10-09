package webhook

import (
	"encoding/json"
	"fmt"
)

// forgejoActions maps Forgejo's and Gitea's action spellings to GitHub's,
// so what acts on an action does not need to know which forge sent it.
// They send label_updated for a label added or removed alike, with no
// delta in the payload, so labeled here means the labels changed; only
// label_cleared is known to remove one.
var forgejoActions = map[string]string{
	"synchronized":  "synchronize",
	"label_updated": "labeled",
	"label_cleared": "unlabeled",
}

// parseForgejo parses a Forgejo or Gitea delivery by its X-Gitea-Event.
// Their finer event types fold into the event names they share with
// GitHub: a label, assignee or milestone change of a pull request arrives
// as pull_request, told apart by its action (see forgejoActions), and a
// comment on an issue or a pull request, a reply in a review's code
// conversation among them, as issue_comment. A review submitted arrives as
// pull_request_approved, pull_request_rejected or pull_request_comment,
// whose payloads name the review's body but none of its code comments.
func parseForgejo(event, delivery string, body []byte) (Event, error) {
	switch event {
	case evPullRequest:
		ev, err := parsePullRequestEvent(delivery, body)
		if a, ok := forgejoActions[ev.Action]; ok {
			ev.Action = a
		}
		return ev, err
	case evIssues:
		ev, err := parseIssueEvent(delivery, body)
		if a, ok := forgejoActions[ev.Action]; ok {
			ev.Action = a
		}
		return ev, err
	case evIssueComment:
		return parseForgejoComment(delivery, body)
	case evPush:
		return parsePush(delivery, body)
	case evRepository:
		// Forgejo and Gitea send created and deleted, the two actions
		// repositoryActions shares with GitHub, and no changes.
		return parseRepository(delivery, body)
	default:
		// The review events, and the rest kritique does not act on.
		return Event{Kind: KindIgnored, Delivery: delivery, Action: event}, nil
	}
}

// parseForgejoComment decodes an issue_comment, whose payload names no
// inline review comment apart from a conversation one: a reply in a code
// conversation is delivered as this event too, with its own comment id, so
// it parses as a conversation comment and the forge client tells the two
// apart when it fetches the comment.
func parseForgejoComment(delivery string, body []byte) (Event, error) {
	var p struct {
		Action     string `json:"action"`
		Repository ghRepo `json:"repository"`
		Issue      struct {
			Number int `json:"number"`
		} `json:"issue"`
		Comment struct {
			ID   int64  `json:"id"`
			Body string `json:"body"`
			User ghUser `json:"user"`
		} `json:"comment"`
		IsPull bool `json:"is_pull"`
	}
	if err := json.Unmarshal(body, &p); err != nil {
		return Event{}, fmt.Errorf("webhook: issue_comment payload: %w", err)
	}
	subject := &Subject{Kind: SubjectIssue, Number: p.Issue.Number}
	if p.IsPull {
		subject.Kind = SubjectPull
	}
	return Event{
		Kind: KindComment, Action: p.Action, Delivery: delivery,
		Repository: p.Repository.event(), Account: p.Repository.Owner.Login,
		Subject: subject,
		Comment: &Comment{
			ID: p.Comment.ID, Number: p.Issue.Number, Author: p.Comment.User.Login,
			AuthorIsBot: p.Comment.User.isBot(), Body: p.Comment.Body,
		},
	}, nil
}
