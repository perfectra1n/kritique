package forgejo

import (
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/perfectra1n/kritique/internal/configfile"
)

func TestLabels(t *testing.T) {
	f, c := newFake(t, configfile.ForgeGitea)
	f.reply("GET /repos/o/r/labels", 200, `[{"id":1,"name":"bug"},{"id":2,"name":"triage"}]`)
	f.reply("POST /repos/o/r/issues/4/labels", 200, `[]`)
	f.reply("GET /repos/o/r/issues/4/labels", 200, `[{"id":2,"name":"triage"}]`)
	f.reply("DELETE /repos/o/r/issues/4/labels/2", 204, ``)
	names, err := c.RepoLabels(t.Context(), "o", "r")
	if err != nil || !reflect.DeepEqual(names, []string{"bug", "triage"}) {
		t.Fatalf("RepoLabels = %v, %v", names, err)
	}
	if err := c.AddLabels(t.Context(), "o", "r", 4, []string{"triage", "bug"}); err != nil {
		t.Fatal(err)
	}
	if b := f.body("POST /repos/o/r/issues/4/labels"); !reflect.DeepEqual(b["labels"], []any{float64(2), float64(1)}) {
		t.Fatalf("labels added by %v, want their ids", b["labels"])
	}
	if err := c.AddLabels(t.Context(), "o", "r", 4, []string{"nope"}); err == nil || !strings.Contains(err.Error(), `no label "nope"`) {
		t.Fatalf("an unknown label = %v", err)
	}
	if err := c.RemoveLabel(t.Context(), "o", "r", 4, "triage"); err != nil || !f.asked("DELETE /repos/o/r/issues/4/labels/2") {
		t.Fatalf("RemoveLabel = %v", err)
	}
	if err := c.RemoveLabel(t.Context(), "o", "r", 4, "bug"); err != nil || f.asked("DELETE /repos/o/r/issues/4/labels/1") {
		t.Fatalf("removing a label not applied = %v", err)
	}
}

func TestTriage(t *testing.T) {
	f, c := newFake(t, configfile.ForgeForgejo)
	f.reply("GET /repos/o/r/issues/4", 200, `{"number":4,"assignees":[{"login":"a"}]}`)
	f.reply("PATCH /repos/o/r/issues/4", 201, `{}`)
	f.reply("POST /repos/o/r/pulls/4/requested_reviewers", 201, `[]`)
	f.mux.HandleFunc("GET /repos/o/r/issues", func(w http.ResponseWriter, r *http.Request) {
		if q := r.URL.Query(); q.Get("q") != "flaky test" || q.Get("limit") != "2" {
			t.Errorf("query = %v", q)
		}
		_, _ = w.Write([]byte(`[{"number":1,"title":"a"},{"number":2,"pull_request":{}},{"number":3}]`))
	})
	if err := c.AddAssignees(t.Context(), "o", "r", 4, []string{"A", "b"}); err != nil {
		t.Fatal(err)
	}
	if b := f.body("PATCH /repos/o/r/issues/4"); !reflect.DeepEqual(b["assignees"], []any{"a", "b"}) {
		t.Fatalf("assignees = %v, want the merged set", b["assignees"])
	}
	if err := c.SetState(t.Context(), "o", "r", 4, false); err != nil {
		t.Fatal(err)
	}
	if b := f.body("PATCH /repos/o/r/issues/4"); b["state"] != "closed" || b["assignees"] != nil {
		t.Fatalf("state edit = %v", b)
	}
	if err := c.RequestReviewers(t.Context(), "o", "r", 4, []string{"dev"}); err != nil {
		t.Fatal(err)
	}
	got, err := c.SearchIssues(t.Context(), "o", "r", "flaky test", 2)
	if err != nil || len(got) != 2 || got[0].Number != 1 || !got[1].PullRequest {
		t.Fatalf("SearchIssues = %+v, %v", got, err)
	}
}
