package ingest

import (
	"testing"

	"github.com/perfectra1n/kritique/internal/configfile"
	"github.com/perfectra1n/kritique/internal/configfile/configfiletest"
	"github.com/perfectra1n/kritique/internal/webhook"
)

// TestDispatchIgnoresNonPullComments runs without a store: a comment on
// anything but a pull request must be turned away before it is touched.
func TestDispatchIgnoresNonPullComments(t *testing.T) {
	svc := NewService(nil, nil)
	tests := []struct {
		name    string
		subject *webhook.Subject
		want    string
	}{
		{"issue comment", &webhook.Subject{Kind: webhook.SubjectIssue, Number: 7}, webhook.SubjectIssue},
		{"unknown subject", &webhook.Subject{Kind: "discussion", Number: 7}, "discussion"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ev := webhook.Event{
				Kind: webhook.KindComment, Action: "created", Subject: tt.subject,
				Repository: &webhook.Repository{FullName: "onedr0p/home-ops"},
				Comment:    &webhook.Comment{ID: 1, Number: 7, Author: "devin", Body: "@bot-ross triage this"},
			}
			out, err := svc.Dispatch(t.Context(), Request{Event: ev})
			if err != nil || out.Status != Ignored || out.Reason != tt.want {
				t.Fatalf("out = %+v, %v", out, err)
			}
		})
	}
}

// TestDispatchTasksNeedNoStoreWithoutACandidate: with tasks off, or no
// task that could run on the event, a delivery never reaches the store.
func TestDispatchTasksNeedNoStoreWithoutACandidate(t *testing.T) {
	t.Setenv("TEST_PEM", "pem")
	t.Setenv("TEST_SECRET", "s3cret")
	off := configfiletest.Load(t, configYAML)
	onlyReleases := configfiletest.Load(t, `taskBounds: { enabled: true, repository: false }
tasks: [{ name: notes, on: [{ raw: { event: release } }] }]
`+configYAML)
	svc := NewService(nil, nil)
	for _, f := range []*configfile.File{off, onlyReleases} {
		account, _ := f.Account(configfile.ForgeGitHub, "onedr0p")
		conn, _ := f.Connection("bot-ross")
		ev := webhook.Event{
			Kind: webhook.KindIssue, Action: "opened", RawEvent: "issues", Sender: "devin",
			Repository: &webhook.Repository{FullName: "onedr0p/home-ops"}, Subject: &webhook.Subject{Kind: webhook.SubjectIssue, Number: 7},
			Issue: &webhook.Issue{Number: 7},
		}
		out, err := svc.Dispatch(t.Context(), Request{File: f, Account: account, Connection: conn, Event: ev})
		if err != nil || out.Status != Ignored {
			t.Fatalf("out = %+v, %v", out, err)
		}
	}
}
