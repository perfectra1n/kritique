package repoconfig

import (
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/perfectra1n/kritique/internal/configfile"
	"github.com/perfectra1n/kritique/internal/tasks"
)

// recipeFiles parses the recipe in docs/recipes/dir and reads the files
// its tasks name.
func recipeFiles(t *testing.T, dir string) (doc []byte, f File, files map[string][]byte) {
	t.Helper()
	root := filepath.Join("../../docs/recipes", dir)
	doc, err := os.ReadFile(filepath.Join(root, FileName))
	if err != nil {
		t.Fatal(err)
	}
	if f, err = Parse(doc); err != nil {
		t.Fatal(err)
	}
	files = map[string][]byte{}
	for i := range f.Tasks {
		for _, p := range f.Tasks[i].Files() {
			if files[p], err = os.ReadFile(filepath.Join(root, p)); err != nil {
				t.Fatal(err)
			}
		}
	}
	return doc, f, files
}

// TestRecipes checks the task recipes docs/tasks.md shows: each file is
// included in the reference, parses, prepares against its templates,
// survives bounds that allow what it uses, and fires on the event it is
// written for.
func TestRecipes(t *testing.T) {
	t.Parallel()
	ref, err := os.ReadFile("../../docs/tasks.md")
	if err != nil {
		t.Fatal(err)
	}
	issue := tasks.SampleInput()
	pull := tasks.SampleInput()
	pull.Event, pull.RawEvent = tasks.EventPullRequest, "pull_request"
	pull.Subject = &tasks.Subject{Kind: tasks.SubjectPull, Number: 2, Title: "Add a widget", State: "open", Author: "octocat"}
	release := tasks.SampleInput()
	release.Event, release.RawEvent, release.Action, release.Subject = "", "release", "published", nil
	release.Raw = map[string]any{"action": "published", "release": map[string]any{"tag_name": "v1.2.0"}}

	tests := []struct {
		dir    string
		events []string
		in     tasks.Input
	}{
		{"issue-triage", tasks.DefaultEvents, issue},
		{"pr-area-labels", tasks.DefaultEvents, pull},
		{"release-summary", []string{"raw:release.*"}, release},
	}
	for _, tt := range tests {
		t.Run(tt.dir, func(t *testing.T) {
			t.Parallel()
			doc, f, files := recipeFiles(t, tt.dir)
			for _, p := range append([]string{FileName}, slices.Sorted(maps.Keys(files))...) {
				if snippet := `--8<-- "docs/recipes/` + tt.dir + "/" + p + `"`; !strings.Contains(string(ref), snippet) {
					t.Errorf("docs/tasks.md does not include docs/recipes/%s/%s", tt.dir, p)
				}
			}
			for i := range f.Tasks {
				if _, err := tasks.Prepare(&f.Tasks[i], files); err != nil {
					t.Fatal(err)
				}
			}

			op := adminSettings()
			op.TaskBounds = (&configfile.TaskBounds{Enabled: new(true), Events: tt.events}).Resolve()
			m, err := Merge(doc, op)
			if err != nil {
				t.Fatal(err)
			}
			if len(m.TaskNotes) > 0 || len(m.Tasks) != len(f.Tasks) {
				t.Fatalf("clipped under the bounds the reference names: %v", m.TaskNotes)
			}
			for _, task := range m.Tasks {
				if ok, err := task.Matches(tt.in); err != nil || !ok {
					t.Errorf("%s does not fire on %s.%s: %v", task.Name, tt.in.RawEvent, tt.in.Action, err)
				}
			}
		})
	}
}

// TestRecipeTriagePlan checks the triage recipe turns an answer into the
// labels and comment the reference says it does.
func TestRecipeTriagePlan(t *testing.T) {
	t.Parallel()
	_, f, files := recipeFiles(t, "issue-triage")
	p, err := tasks.Prepare(&f.Tasks[0], files)
	if err != nil {
		t.Fatal(err)
	}
	a, err := p.ParseAnswer([]byte(`{"summary": "Crash on start.", "comment": "",
		"fields": {"priority": "p1", "area": "web", "needsInfo": true, "missing": ["the version"]},
		"labels": {"add": ["bug"], "remove": ["needs-triage"]}}`))
	if err != nil {
		t.Fatal(err)
	}
	in := tasks.SampleInput()
	in.Subject.Labels = []string{"needs-triage"}
	labels := []string{"bug", "enhancement", "needs-triage", "needs-info", "triaged", "priority/p1", "area/web"}
	plan, err := p.Plan(in, a, tasks.Facts{RepoLabels: labels, Subject: in.Subject})
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"bug", "priority/p1", "area/web", "triaged", "needs-info"}; !slices.Equal(plan.AddLabels, want) {
		t.Errorf("AddLabels = %v, want %v (dropped %v)", plan.AddLabels, want, plan.Dropped)
	}
	if want := []string{"needs-triage"}; !slices.Equal(plan.RemoveLabels, want) {
		t.Errorf("RemoveLabels = %v, want %v", plan.RemoveLabels, want)
	}
	if plan.Comment == nil || plan.Comment.Mode != tasks.CommentSticky || !strings.Contains(plan.Comment.Body, "- the version") {
		t.Errorf("comment = %+v", plan.Comment)
	}

	// Without needsInfo or a comment, no when condition of the comment
	// holds.
	a.Fields["needsInfo"] = false
	plan, err = p.Plan(in, a, tasks.Facts{RepoLabels: labels, Subject: in.Subject})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Comment != nil {
		t.Errorf("comment = %+v, want none", plan.Comment)
	}
}
