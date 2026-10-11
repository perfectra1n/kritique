package webhook

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/perfectra1n/kritique/internal/configfile"
)

// fjPullRequest is a Forgejo pull_request delivery, trimmed to what the
// parser reads: a user carries no type, and the action is Forgejo's own.
const fjPullRequest = `{
  "action": "synchronized",
  "number": 7,
  "pull_request": {
    "number": 7, "title": "fix: thing", "body": "b", "state": "open", "draft": true, "merged": false,
    "html_url": "https://git.example.com/perfectra1n/home/pulls/7", "created_at": "2026-10-01T10:00:00Z",
    "updated_at": "2026-10-01T11:00:00Z", "closed_at": null,
    "user": {"login": "perfectra1n"},
    "head": {"ref": "fix", "sha": "aaa111", "repo": {"full_name": "perfectra1n/home"}},
    "base": {"ref": "main", "sha": "bbb222", "repo": {"full_name": "perfectra1n/home"}},
    "labels": [{"name": "bug", "color": "ee0701"}]
  },
  "repository": {"full_name": "perfectra1n/home", "default_branch": "main", "archived": false, "fork": false,
    "owner": {"login": "perfectra1n"}}
}`

func gitea(event string) http.Header {
	h := http.Header{}
	h.Set("X-Gitea-Event", event)
	h.Set("X-Gitea-Delivery", "g-1")
	h.Set("Content-Type", "application/json")
	return h
}

func TestParseForgejoPullRequest(t *testing.T) {
	for _, forge := range []configfile.Forge{configfile.ForgeForgejo, configfile.ForgeGitea} {
		t.Run(string(forge), func(t *testing.T) {
			ev, err := Parse(forge, gitea("pull_request"), []byte(fjPullRequest))
			if err != nil {
				t.Fatal(err)
			}
			if ev.Kind != KindPullRequest || ev.Action != "synchronize" || ev.Delivery != "g-1" || ev.Account != "perfectra1n" {
				t.Fatalf("event = %+v", ev)
			}
			pr := ev.PullRequest
			if pr.Number != 7 || pr.HeadSHA != "aaa111" || pr.BaseRef != "main" || !pr.Draft || pr.Fork || pr.AuthorIsBot ||
				pr.ClosedAt != nil || !pr.UpdatedAt.Equal(time.Date(2026, 10, 1, 11, 0, 0, 0, time.UTC)) {
				t.Fatalf("pr = %+v", pr)
			}
			if len(pr.Labels) != 1 || pr.Labels[0].Name != "bug" {
				t.Fatalf("labels = %v", pr.Labels)
			}
			if ev.Repository.FullName != "perfectra1n/home" || ev.Repository.DefaultBranch != "main" {
				t.Fatalf("repo = %+v", ev.Repository)
			}
		})
	}
}

// TestParseForgejoActions: Forgejo's action spellings read as GitHub's,
// and only on a Forgejo or Gitea delivery.
func TestParseForgejoActions(t *testing.T) {
	tests := []struct{ sent, want string }{
		{"synchronized", "synchronize"},
		{"label_updated", "labeled"},
		{"label_cleared", "unlabeled"},
		{"opened", "opened"},
		{"reopened", "reopened"},
		{"closed", "closed"},
		{"edited", "edited"},
	}
	for _, tt := range tests {
		t.Run(tt.sent, func(t *testing.T) {
			body := strings.Replace(fjPullRequest, `"action": "synchronized"`, `"action": "`+tt.sent+`"`, 1)
			ev, err := Parse(configfile.ForgeForgejo, gitea("pull_request"), []byte(body))
			if err != nil || ev.Action != tt.want {
				t.Fatalf("action = %q, %v; want %q", ev.Action, err, tt.want)
			}
		})
	}
	ev, err := Parse(configfile.ForgeGitHub, gh("pull_request"), []byte(fjPullRequest))
	if err != nil || ev.Action != "synchronized" {
		t.Fatalf("a GitHub delivery's action = %q, %v; want it left as sent", ev.Action, err)
	}
}

func TestParseForgejoComments(t *testing.T) {
	const pull = `{"action":"created","is_pull":true,"issue":{"number":3},
	  "comment":{"id":99,"body":"@kritique why?","user":{"login":"perfectra1n"}},
	  "repository":{"full_name":"perfectra1n/home","owner":{"login":"perfectra1n"}}}`
	ev, err := Parse(configfile.ForgeForgejo, gitea("issue_comment"), []byte(pull))
	if err != nil || ev.Kind != KindComment || ev.Action != "created" || ev.Account != "perfectra1n" {
		t.Fatalf("comment = %+v, %v", ev, err)
	}
	c := ev.Comment
	// A reply in a code conversation is delivered the same way: the forge
	// client tells it apart when it fetches the comment.
	if c.ID != 99 || c.Number != 3 || c.Author != "perfectra1n" || c.Inline || c.AuthorIsBot || c.Body != "@kritique why?" {
		t.Fatalf("comment = %+v", c)
	}
	issue := strings.Replace(pull, `"is_pull":true`, `"is_pull":false`, 1)
	// A plain issue's comment is offered to tasks, under an issue subject
	// so it is never taken for a review follow-up.
	if ev, err := Parse(configfile.ForgeGitea, gitea("issue_comment"), []byte(issue)); err != nil ||
		ev.Kind != KindComment || ev.Subject == nil || ev.Subject.Kind != SubjectIssue || ev.Subject.Number != 3 {
		t.Fatalf("an issue's comment = %+v, %v; want a comment on issue 3", ev, err)
	}
	if ev.Subject == nil || ev.Subject.Kind != SubjectPull {
		t.Fatalf("a pull request's comment subject = %+v; want the pull request", ev.Subject)
	}
	if _, err := Parse(configfile.ForgeForgejo, gitea("issue_comment"), []byte("{")); err == nil {
		t.Fatal("a broken payload must be an error")
	}
}

func TestParseForgejoOther(t *testing.T) {
	repo := `"repository":{"full_name":"perfectra1n/home","default_branch":"main","owner":{"login":"perfectra1n"}}`
	tests := []struct {
		name, event, body string
		kind              Kind
		action            string
	}{
		{"push", "push", `{"ref":"refs/heads/main","after":"ccc333",` + repo + `}`, KindPush, ""},
		{"repository created", "repository", `{"action":"created",` + repo + `}`, KindRepository, "created"},
		{"repository deleted", "repository", `{"action":"deleted",` + repo + `}`, KindRepository, "deleted"},
		{"an issue", "issues", `{"action":"opened","issue":{"number":4,"pull_request":null},` + repo + `}`, KindIssue, "opened"},
		{"an issue's labels", "issues", `{"action":"label_updated","issue":{"number":4},` + repo + `}`, KindIssue, "labeled"},
		// An ignored event keeps the payload's action; its name is RawEvent.
		{"a review approving", "pull_request_approved", `{"action":"reviewed",` + repo + `}`, KindIgnored, "reviewed"},
		{"a review commenting", "pull_request_comment", `{"action":"reviewed",` + repo + `}`, KindIgnored, "reviewed"},
		{"a release", "release", `{"action":"published",` + repo + `}`, KindIgnored, "published"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ev, err := Parse(configfile.ForgeForgejo, gitea(tt.event), []byte(tt.body))
			if err != nil || ev.Kind != tt.kind || ev.Action != tt.action || ev.RawEvent != tt.event {
				t.Fatalf("event = %+v, %v; want kind %s action %q raw event %q", ev, err, tt.kind, tt.action, tt.event)
			}
		})
	}
	ev, _ := Parse(configfile.ForgeForgejo, gitea("push"), []byte(tests[0].body))
	if ev.Push.Ref != "refs/heads/main" || ev.Push.After != "ccc333" || ev.Repository.FullName != "perfectra1n/home" {
		t.Fatalf("push = %+v %+v", ev.Push, ev.Repository)
	}
}

// TestParseForgejoForm: a delivery sent as a form wraps the JSON in a
// payload field.
func TestParseForgejoForm(t *testing.T) {
	h := gitea("issue_comment")
	h.Set("Content-Type", "application/x-www-form-urlencoded")
	body := "payload=" + strings.NewReplacer("{", "%7B", "}", "%7D", `"`, "%22", ":", "%3A", ",", "%2C", " ", "+").Replace(
		`{"action":"created","is_pull":true,"issue":{"number":1},"comment":{"id":5,"body":"hi","user":{"login":"x"}},`+
			`"repository":{"full_name":"a/b","owner":{"login":"a"}}}`)
	ev, err := Parse(configfile.ForgeForgejo, h, []byte(body))
	if err != nil || ev.Kind != KindComment || ev.Comment.ID != 5 {
		t.Fatalf("form delivery = %+v, %v", ev, err)
	}
}
