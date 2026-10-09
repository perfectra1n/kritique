package forgejo

import (
	"testing"

	"github.com/perfectra1n/kritique/internal/configfile"
)

func TestApprovals(t *testing.T) {
	const reviews = `[
		{"id":1,"user":{"login":"kritique"},"state":"APPROVED","commit_id":"old"},
		{"id":2,"user":{"login":"kritique"},"state":"APPROVED","commit_id":"older","dismissed":true},
		{"id":3,"user":{"login":"dev"},"state":"REQUEST_CHANGES"},
		{"id":4,"user":{"login":"dev"},"state":"COMMENT"},
		{"id":5,"user":{"login":"ops"},"state":"REQUEST_CHANGES","dismissed":true}]`
	t.Run("approve a new head", func(t *testing.T) {
		f, c := newFake(t, configfile.ForgeForgejo)
		f.reply("GET /user", 200, `{"login":"kritique"}`)
		f.reply("GET /repos/o/r/pulls/4/reviews", 200, reviews)
		f.reply("POST /repos/o/r/pulls/4/reviews", 200, `{"id":6}`)
		if ok, err := c.Approve(t.Context(), "o", "r", 4, "old", "fine"); ok || err != nil {
			t.Fatalf("Approve of an approved head = %v, %v", ok, err)
		}
		if ok, err := c.Approve(t.Context(), "o", "r", 4, "new", "fine"); !ok || err != nil {
			t.Fatalf("Approve = %v, %v", ok, err)
		}
		if b := f.body("POST /repos/o/r/pulls/4/reviews"); b["event"] != "APPROVED" || b["commit_id"] != "new" || b["body"] != "fine" {
			t.Fatalf("approval = %v", b)
		}
	})
	t.Run("changes requested", func(t *testing.T) {
		f, c := newFake(t, configfile.ForgeForgejo)
		f.reply("GET /user", 200, `{"login":"kritique"}`)
		f.reply("GET /repos/o/r/pulls/4/reviews", 200, reviews)
		if ok, err := c.ChangesRequested(t.Context(), "o", "r", 4); !ok || err != nil {
			t.Fatalf("ChangesRequested = %v, %v", ok, err)
		}
		f2, c2 := newFake(t, configfile.ForgeForgejo)
		f2.reply("GET /user", 200, `{"login":"kritique"}`)
		f2.reply("GET /repos/o/r/pulls/4/reviews", 200, `[{"id":3,"user":{"login":"dev"},"state":"REQUEST_CHANGES"},
			{"id":7,"user":{"login":"dev"},"state":"APPROVED"},{"id":8,"user":{"login":"kritique"},"state":"REQUEST_CHANGES"}]`)
		if ok, err := c2.ChangesRequested(t.Context(), "o", "r", 4); ok || err != nil {
			t.Fatalf("a request since approved, and the bot's own = %v, %v", ok, err)
		}
	})
	t.Run("dismiss, or delete without admin", func(t *testing.T) {
		f, c := newFake(t, configfile.ForgeGitea)
		f.reply("GET /user", 200, `{"login":"kritique"}`)
		f.reply("GET /repos/o/r/pulls/4/reviews", 200, `[
			{"id":1,"user":{"login":"kritique"},"state":"APPROVED"},{"id":9,"user":{"login":"kritique"},"state":"APPROVED"}]`)
		f.reply("POST /repos/o/r/pulls/4/reviews/1/dismissals", 200, `{}`)
		f.reply("POST /repos/o/r/pulls/4/reviews/9/dismissals", 403, `{"message":"Must be repo admin"}`)
		f.reply("DELETE /repos/o/r/pulls/4/reviews/9", 204, ``)
		n, err := c.DismissApprovals(t.Context(), "o", "r", 4, "a later review found blocking issues")
		if err != nil || n != 2 || !f.asked("DELETE /repos/o/r/pulls/4/reviews/9") {
			t.Fatalf("DismissApprovals = %d, %v; requests %v", n, err, f.sent())
		}
		if b := f.body("POST /repos/o/r/pulls/4/reviews/1/dismissals"); b["message"] != "a later review found blocking issues" {
			t.Fatalf("dismissal = %v", b)
		}
	})
}
