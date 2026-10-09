package webapi

import (
	"testing"

	"github.com/perfectra1n/kritique/internal/configfile"
)

func TestCommentURL(t *testing.T) {
	const gh, fj = "https://github.com/alpha/one/pull/7", "https://git.example/alpha/one/pulls/7"
	for _, tt := range []struct {
		name    string
		forge   configfile.Forge
		pullURL string
		id      int64
		inline  bool
		want    string
	}{
		{"github conversation", configfile.ForgeGitHub, gh, 9, false, gh + "#issuecomment-9"},
		{"github diff thread", configfile.ForgeGitHub, gh, 9, true, gh + "#discussion_r9"},
		{"forgejo conversation", "forgejo", fj, 9, false, fj + "#issuecomment-9"},
		{"forgejo diff comment", "forgejo", fj, 9, true, fj + "#issuecomment-9"},
		{"gitea diff comment", "gitea", fj, 9, true, fj + "#issuecomment-9"},
		{"no comment", configfile.ForgeGitHub, gh, 0, true, ""},
		{"no pull request", configfile.ForgeGitHub, "", 9, false, ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := commentURL(tt.forge, tt.pullURL, tt.id, tt.inline); got != tt.want {
				t.Fatalf("commentURL = %q, want %q", got, tt.want)
			}
		})
	}
}
