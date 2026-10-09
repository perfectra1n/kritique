package forgejo

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/perfectra1n/kritique/internal/configfile"
	"github.com/perfectra1n/kritique/internal/forge"
)

func pullJSON(number int, head, mergeBase string) string {
	return fmt.Sprintf(`{"number":%d,"state":"open","merge_base":%q,"title":"t","user":{"login":"dev"},
		"html_url":"https://git.example.com/o/r/pulls/%d","updated_at":"2026-10-01T10:00:00Z",
		"head":{"ref":"feat","sha":%q,"repo":{"full_name":"o/r"}},
		"base":{"ref":"main","sha":"b","repo":{"full_name":"o/r","default_branch":"main"}},
		"additions":3,"deletions":1,"changed_files":2,"labels":[{"name":"bug","color":"ee0701"}]}`, number, mergeBase, number, head)
}

func TestMergeBase(t *testing.T) {
	t.Run("the pull request the context names", func(t *testing.T) {
		f, c := newFake(t, configfile.ForgeForgejo)
		f.reply("GET /repos/o/r/pulls/7", 200, pullJSON(7, "head1", "base1"))
		got, err := c.MergeBase(forge.WithPullRequest(t.Context(), 7), "o", "r", "main", "head1")
		if err != nil || got != "base1" || f.asked("GET /repos/o/r/pulls") {
			t.Fatalf("MergeBase = %q, %v; requests %v", got, err, f.sent())
		}
	})
	t.Run("found by listing, then remembered", func(t *testing.T) {
		f, c := newFake(t, configfile.ForgeForgejo)
		f.mux.HandleFunc("GET /repos/o/r/pulls", func(w http.ResponseWriter, r *http.Request) {
			q := r.URL.Query()
			if q.Get("state") != "open" || q.Get("sort") != "recentupdate" {
				t.Errorf("query = %v", q)
			}
			_, _ = fmt.Fprintf(w, "[%s,%s]", pullJSON(9, "other", "x"), pullJSON(8, "head1", "base1"))
		})
		f.reply("GET /repos/o/r/pulls/8", 200, pullJSON(8, "head1", "base1"))
		for range 2 {
			if got, err := c.MergeBase(t.Context(), "o", "r", "main", "head1"); err != nil || got != "base1" {
				t.Fatalf("MergeBase = %q, %v", got, err)
			}
		}
		listed := 0
		for _, r := range f.sent() {
			if r == "GET /repos/o/r/pulls" {
				listed++
			}
		}
		if listed != 1 {
			t.Fatalf("listed %d times, want once; requests %v", listed, f.sent())
		}
	})
	t.Run("a head no open pull request has", func(t *testing.T) {
		f, c := newFake(t, configfile.ForgeForgejo)
		f.reply("GET /repos/o/r/pulls/7", 200, pullJSON(7, "newer", "base1"))
		f.reply("GET /repos/o/r/pulls", 200, "["+pullJSON(7, "newer", "base1")+"]")
		if _, err := c.MergeBase(forge.WithPullRequest(t.Context(), 7), "o", "r", "main", "head1"); err == nil {
			t.Fatal("a pull request that moved on must not answer for its old head")
		}
	})
	t.Run("another base branch", func(t *testing.T) {
		f, c := newFake(t, configfile.ForgeForgejo)
		f.reply("GET /repos/o/r/pulls/7", 200, pullJSON(7, "head1", "base1"))
		f.reply("GET /repos/o/r/pulls", 200, `[]`)
		if _, err := c.MergeBase(forge.WithPullRequest(t.Context(), 7), "o", "r", "release", "head1"); err == nil {
			t.Fatal("a pull request into another branch must not answer")
		}
	})
}

func TestPullRequestDiff(t *testing.T) {
	const diff = "diff --git a/a.go b/a.go\n+b\n"
	f, c := newFake(t, configfile.ForgeForgejo)
	f.reply("GET /repos/o/r/pulls/7", 200, pullJSON(7, "head1", "base1"))
	f.mux.HandleFunc("GET /repos/o/r/pulls/7.diff", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(diff)) })
	ctx := forge.WithPullRequest(t.Context(), 7)
	if got, err := c.PullRequestDiff(ctx, "o", "r", "base1", "head1"); err != nil || got != diff {
		t.Fatalf("PullRequestDiff = %q, %v", got, err)
	}
	if _, err := c.PullRequestDiff(ctx, "o", "r", "elsewhere", "head1"); err == nil {
		t.Fatal("a diff from another base must be an error, never the pull request's")
	}
	g, d := newFake(t, configfile.ForgeForgejo)
	g.reply("GET /repos/o/r/pulls/7", 200, pullJSON(7, "head1", "base1"))
	g.mux.HandleFunc("GET /repos/o/r/pulls/7.diff", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(bytes.Repeat([]byte("x"), maxDiffBytes+1))
	})
	if got, err := d.PullRequestDiff(ctx, "o", "r", "base1", "head1"); err == nil {
		t.Fatalf("PullRequestDiff = %d bytes, want an error over the cap", len(got))
	}
}

func TestListOpenPullRequestsAndPullRequest(t *testing.T) {
	f, c := newFake(t, configfile.ForgeGitea)
	var (
		mu    sync.Mutex
		pages []string
	)
	f.mux.HandleFunc("GET /repos/o/r/pulls", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		pages = append(pages, r.URL.Query().Get("page"))
		mu.Unlock()
		if r.URL.Query().Get("page") == "1" {
			items := make([]string, pageSize)
			for i := range items {
				items[i] = pullJSON(100+i, "h", "b")
			}
			_, _ = w.Write([]byte("[" + strings.Join(items, ",") + "]"))
			return
		}
		_, _ = w.Write([]byte(`[{"number":1,"state":"open","head":{"sha":"x","repo":null},"base":{"ref":"main","repo":{"full_name":"o/r"}},
			"updated_at":"2026-01-01T00:00:00Z","user":{"login":"renovate[bot]"}}]`))
	})
	prs, err := c.ListOpenPullRequests(t.Context(), "o", "r", time.Time{})
	mu.Lock()
	got := strings.Join(pages, ",")
	mu.Unlock()
	if err != nil || len(prs) != pageSize+1 || got != "1,2" {
		t.Fatalf("ListOpenPullRequests = %d, %v; pages %v", len(prs), err, pages)
	}
	pr := prs[0]
	if pr.Number != 100 || pr.HeadSHA != "h" || pr.BaseRef != "main" || pr.DefaultBranch != "main" || pr.Fork || len(pr.Labels) != 1 ||
		pr.UpdatedAt.IsZero() {
		t.Fatalf("pr = %+v", pr)
	}
	if last := prs[pageSize]; !last.Fork || !last.AuthorIsBot {
		t.Fatalf("a deleted fork's bot pull request = %+v", last)
	}
	recent, err := c.ListOpenPullRequests(t.Context(), "o", "r", time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC))
	if err != nil || len(recent) != pageSize {
		t.Fatalf("since a time = %d, %v; want the walk to stop at the older one", len(recent), err)
	}

	f.reply("GET /repos/o/r/pulls/5", 200, pullJSON(5, "h", "b"))
	f.reply("GET /repos/o/r/pulls/6", 404, `{}`)
	one, err := c.PullRequest(t.Context(), "o", "r", 5)
	if err != nil || one.Additions != 3 || one.Deletions != 1 || one.ChangedFiles != 2 {
		t.Fatalf("PullRequest = %+v, %v", one, err)
	}
	if _, err := c.PullRequest(t.Context(), "o", "r", 6); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("PullRequest of an unknown number = %v", err)
	}
}
