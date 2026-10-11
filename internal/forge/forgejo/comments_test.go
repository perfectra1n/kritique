package forgejo

import (
	"errors"
	"io/fs"
	"net/http"
	"testing"

	"github.com/perfectra1n/kritique/internal/configfile"
	"github.com/perfectra1n/kritique/internal/forge"
)

// reviewFixture is pull request 4 with two reviews: the bot's, posting two
// findings, a person's reply in the first finding's conversation, and a
// person's own review.
func reviewFixture(f *fakeAPI) {
	f.reply("GET /user", 200, `{"login":"kritique"}`)
	f.reply("GET /repos/o/r/pulls/4/reviews", 200, `[
		{"id":20,"user":{"login":"dev"},"state":"COMMENT"},
		{"id":10,"user":{"login":"kritique"},"state":"COMMENT","commit_id":"h1"}]`)
	f.reply("GET /repos/o/r/pulls/4/reviews/10/comments", 200, `[
		{"id":103,"pull_request_review_id":10,"path":"a.go","position":5,"body":"thanks, fixed","user":{"login":"dev"},
		 "created_at":"2026-10-01T10:05:00Z"},
		{"id":101,"pull_request_review_id":10,"path":"a.go","position":5,"body":"<!-- kritique:finding:f1 -->\nleak",
		 "user":{"login":"kritique"},"created_at":"2026-10-01T10:00:00Z"},
		{"id":102,"pull_request_review_id":10,"path":"b.go","original_position":9,"body":"<!-- kritique:finding:f2 -->\nnil",
		 "user":{"login":"kritique"},"created_at":"2026-10-01T10:00:00Z","resolver":{"login":"dev"}}]`)
	f.reply("GET /repos/o/r/pulls/4/reviews/20/comments", 200, `[
		{"id":201,"pull_request_review_id":20,"path":"a.go","position":5,"body":"own note","user":{"login":"dev"},
		 "created_at":"2026-10-01T09:00:00Z"}]`)
	f.reply("GET /repos/o/r/issues/comments/101/reactions", 200, `[{"content":"+1"},{"content":"+1"},{"content":"-1"},{"content":"eyes"}]`)
	f.reply("GET /repos/o/r/issues/comments/102/reactions", 200, `null`)
}

func TestListInline(t *testing.T) {
	f, c := newFake(t, configfile.ForgeForgejo)
	reviewFixture(f)
	got, err := c.ListInline(t.Context(), "o", "r", 4)
	if err != nil {
		t.Fatal(err)
	}
	type row struct {
		id, inReplyTo int64
		line          int
		up, down      int
	}
	want := []row{{201, 0, 5, 0, 0}, {101, 0, 5, 2, 1}, {102, 0, 0, 0, 0}, {103, 101, 5, 0, 0}}
	if len(got) != len(want) {
		t.Fatalf("ListInline = %+v", got)
	}
	for i, w := range want {
		g := got[i]
		if (row{g.ID, g.InReplyTo, g.Line, g.ReactionsUp, g.ReactionsDown}) != w || !g.Inline {
			t.Errorf("comment %d = %+v, want %+v", i, g, w)
		}
	}
	// Reactions are read for the bot's roots alone.
	if f.asked("GET /repos/o/r/issues/comments/103/reactions") || f.asked("GET /repos/o/r/issues/comments/201/reactions") {
		t.Fatalf("reactions read for a person's comment: %v", f.sent())
	}
}

func TestGetComment(t *testing.T) {
	f, c := newFake(t, configfile.ForgeForgejo)
	reviewFixture(f)
	f.reply("GET /repos/o/r/issues/comments/55", 200, `{"id":55,"body":"@kritique hi","user":{"login":"dev"}}`)
	f.mux.HandleFunc("GET /repos/o/r/issues/comments/103", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	cm, err := c.GetComment(t.Context(), "o", "r", 55, false)
	if err != nil || cm.ID != 55 || cm.Inline || cm.Author != "dev" {
		t.Fatalf("conversation comment = %+v, %v", cm, err)
	}
	ctx := forge.WithPullRequest(t.Context(), 4)
	// A reply in a code conversation arrives as a conversation comment.
	cm, err = c.GetComment(ctx, "o", "r", 103, false)
	if err != nil || !cm.Inline || cm.InReplyTo != 101 || cm.Path != "a.go" {
		t.Fatalf("code reply = %+v, %v", cm, err)
	}
	if cm, err := c.GetComment(ctx, "o", "r", 101, true); err != nil || cm.InReplyTo != 0 || cm.Author != "kritique" {
		t.Fatalf("inline root = %+v, %v", cm, err)
	}
	if _, err := c.GetComment(ctx, "o", "r", 999, true); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("unknown inline comment = %v", err)
	}
	if _, err := c.GetComment(t.Context(), "o", "r", 101, true); !errors.Is(err, ErrNoPullRequest) {
		t.Fatalf("inline comment without a pull request = %v", err)
	}
}

func TestConversationComments(t *testing.T) {
	f, c := newFake(t, configfile.ForgeForgejo)
	f.reply("GET /repos/o/r/issues/4/comments", 200, `[
		{"id":2,"body":"<!-- kritique:pr-4 -->","user":{"login":"Kritique"},"created_at":"2026-10-01T10:00:00Z"},
		{"id":1,"body":"first","user":{"login":"dev"},"created_at":"2026-10-01T09:00:00Z"}]`)
	f.reply("POST /repos/o/r/issues/4/comments", 201, `{"id":3}`)
	f.reply("PATCH /repos/o/r/issues/comments/2", 200, `{"id":2}`)
	f.reply("PATCH /repos/o/r/issues/comments/9", 404, `{}`)
	list, err := c.ListConversation(t.Context(), "o", "r", 4)
	if err != nil || len(list) != 2 || list[0].ID != 1 {
		t.Fatalf("ListConversation = %+v, %v", list, err)
	}
	if id, err := c.FindComment(t.Context(), "o", "r", 4, "kritique", "<!-- kritique:pr-4 -->"); err != nil || id != 2 {
		t.Fatalf("FindComment = %d, %v", id, err)
	}
	if id, err := c.CreateComment(t.Context(), "o", "r", 4, "hello"); err != nil || id != 3 {
		t.Fatalf("CreateComment = %d, %v", id, err)
	}
	if err := c.UpdateComment(t.Context(), "o", "r", 2, "x"); err != nil {
		t.Fatal(err)
	}
	if err := c.UpdateComment(t.Context(), "o", "r", 9, "x"); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("UpdateComment of a deleted comment = %v", err)
	}
}

func TestCreateReview(t *testing.T) {
	f, c := newFake(t, configfile.ForgeForgejo)
	f.reply("POST /repos/o/r/pulls/4/reviews", 200, `{"id":30}`)
	f.reply("GET /repos/o/r/pulls/4/reviews/30/comments", 200, `[
		{"id":302,"path":"b.go","body":"second"},{"id":301,"path":"a.go","body":"first"}]`)
	ids, err := c.CreateReview(t.Context(), "o", "r", 4, "h1", []forge.InlineComment{
		{Path: "a.go", Line: 7, StartLine: 3, Body: "first"}, {Path: "b.go", Line: 2, Body: "second"}, {Path: "c.go", Line: 1, Body: "lost"},
	})
	if err != nil || len(ids) != 3 || ids[0] != 301 || ids[1] != 302 || ids[2] != 0 {
		t.Fatalf("CreateReview = %v, %v", ids, err)
	}
	b := f.body("POST /repos/o/r/pulls/4/reviews")
	first := b["comments"].([]any)[0].(map[string]any)
	if b["event"] != "COMMENT" || b["commit_id"] != "h1" || first["new_position"] != float64(7) || first["path"] != "a.go" {
		t.Fatalf("review = %v", b)
	}
	if ids, err := c.CreateReview(t.Context(), "o", "r", 4, "h1", nil); err != nil || len(ids) != 0 {
		t.Fatalf("an empty review = %v, %v", ids, err)
	}
}

func TestReplyInline(t *testing.T) {
	root := forge.Comment{ID: 101, Inline: true, Path: "a.go", Line: 5}
	reply := forge.Comment{ID: 103, Inline: true, InReplyTo: 101}
	t.Run("forgejo adds to the root's review", func(t *testing.T) {
		f, c := newFake(t, configfile.ForgeForgejo)
		reviewFixture(f)
		f.reply("POST /repos/o/r/pulls/4/reviews/10/comments", 200, `{"id":104}`)
		id, err := c.ReplyInline(t.Context(), "o", "r", 4, reply, "answer")
		if err != nil || id != 104 {
			t.Fatalf("ReplyInline = %d, %v", id, err)
		}
		b := f.body("POST /repos/o/r/pulls/4/reviews/10/comments")
		if b["path"] != "a.go" || b["new_position"] != float64(5) || b["body"] != "answer" {
			t.Fatalf("reply = %v", b)
		}
	})
	t.Run("gitea replies to the root", func(t *testing.T) {
		f, c := newFake(t, configfile.ForgeGitea)
		reviewFixture(f)
		f.reply("POST /repos/o/r/pulls/4/comments/101/replies", 201, `{"id":105}`)
		if id, err := c.ReplyInline(t.Context(), "o", "r", 4, root, "answer"); err != nil || id != 105 {
			t.Fatalf("ReplyInline = %d, %v", id, err)
		}
	})
	t.Run("a gitea without replies gets a review of its own", func(t *testing.T) {
		f, c := newFake(t, configfile.ForgeGitea)
		reviewFixture(f)
		f.reply("POST /repos/o/r/pulls/4/comments/101/replies", 404, `{}`)
		f.reply("GET /repos/o/r/pulls/4", 200, pullJSON(4, "h2", "b"))
		f.reply("POST /repos/o/r/pulls/4/reviews", 200, `{"id":40}`)
		f.reply("GET /repos/o/r/pulls/4/reviews/40/comments", 200, `[{"id":401}]`)
		if id, err := c.ReplyInline(t.Context(), "o", "r", 4, root, "answer"); err != nil || id != 401 {
			t.Fatalf("ReplyInline = %d, %v", id, err)
		}
		if b := f.body("POST /repos/o/r/pulls/4/reviews"); b["commit_id"] != "h2" {
			t.Fatalf("fallback review = %v", b)
		}
	})
	t.Run("a refusal is an error", func(t *testing.T) {
		f, c := newFake(t, configfile.ForgeForgejo)
		reviewFixture(f)
		f.reply("POST /repos/o/r/pulls/4/reviews/10/comments", 403, `{}`)
		if _, err := c.ReplyInline(t.Context(), "o", "r", 4, root, "answer"); err == nil {
			t.Fatal("want an error")
		}
	})
}

func TestResolveThread(t *testing.T) {
	t.Run("forgejo has no API for it", func(t *testing.T) {
		f, c := newFake(t, configfile.ForgeForgejo)
		ok, err := c.ResolveThread(t.Context(), "o", "r", 4, 101, false)
		if ok || err != nil || len(f.sent()) != 0 {
			t.Fatalf("ResolveThread = %v, %v; requests %v", ok, err, f.sent())
		}
	})
	t.Run("gitea", func(t *testing.T) {
		f, c := newFake(t, configfile.ForgeGitea)
		reviewFixture(f)
		f.reply("POST /repos/o/r/pulls/comments/101/resolve", 204, ``)
		// A person wrote in 101's conversation.
		if ok, err := c.ResolveThread(t.Context(), "o", "r", 4, 101, true); ok || err != nil {
			t.Fatalf("onlyOwn on a conversation = %v, %v", ok, err)
		}
		if ok, err := c.ResolveThread(t.Context(), "o", "r", 4, 101, false); !ok || err != nil {
			t.Fatalf("ResolveThread = %v, %v", ok, err)
		}
		// 102 is resolved already.
		if ok, err := c.ResolveThread(t.Context(), "o", "r", 4, 102, false); ok || err != nil || f.asked("POST /repos/o/r/pulls/comments/102/resolve") {
			t.Fatalf("an already resolved thread = %v, %v", ok, err)
		}
	})
	t.Run("a gitea without the endpoint", func(t *testing.T) {
		f, c := newFake(t, configfile.ForgeGitea)
		reviewFixture(f)
		f.reply("POST /repos/o/r/pulls/comments/101/resolve", 404, `{}`)
		for range 2 {
			if ok, err := c.ResolveThread(t.Context(), "o", "r", 4, 101, false); ok || err != nil {
				t.Fatalf("ResolveThread = %v, %v; want not done", ok, err)
			}
		}
		n := 0
		for _, r := range f.sent() {
			if r == "POST /repos/o/r/pulls/comments/101/resolve" {
				n++
			}
		}
		if n != 1 {
			t.Fatalf("asked %d times, want once", n)
		}
	})
}

func TestReactions(t *testing.T) {
	f, c := newFake(t, configfile.ForgeForgejo)
	f.reply("POST /repos/o/r/issues/comments/7/reactions", 201, `{"content":"eyes"}`)
	f.reply("DELETE /repos/o/r/issues/comments/7/reactions", 200, ``)
	f.reply("POST /repos/o/r/issues/4/reactions", 200, `{"content":"+1"}`)
	f.reply("DELETE /repos/o/r/issues/4/reactions", 200, ``)
	cm := forge.Comment{ID: 7, Inline: true}
	id, err := c.React(t.Context(), "o", "r", cm, forge.ReactionEyes)
	if err != nil || id == 0 {
		t.Fatalf("React = %d, %v", id, err)
	}
	if err := c.Unreact(t.Context(), "o", "r", cm, id); err != nil {
		t.Fatal(err)
	}
	if b := f.body("DELETE /repos/o/r/issues/comments/7/reactions"); b["content"] != "eyes" {
		t.Fatalf("Unreact removed %v, want the eyes", b)
	}
	id, err = c.ReactToPullRequest(t.Context(), "o", "r", 4, forge.ReactionDone)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.UnreactToPullRequest(t.Context(), "o", "r", 4, id); err != nil {
		t.Fatal(err)
	}
	if b := f.body("DELETE /repos/o/r/issues/4/reactions"); b["content"] != "+1" {
		t.Fatalf("UnreactToPullRequest removed %v", b)
	}
	if err := c.Unreact(t.Context(), "o", "r", cm, 99); !errors.Is(err, errReactionID) {
		t.Fatalf("Unreact of an unknown id = %v", err)
	}
	if _, err := c.React(t.Context(), "o", "r", cm, "party"); err == nil {
		t.Fatal("an unknown reaction must be an error")
	}
}
