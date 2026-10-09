package webapi

import (
	"fmt"

	"github.com/perfectra1n/kritique/internal/configfile"
)

// commentURL links comment id on the pull request at pullURL as forge
// anchors it there; inline is whether it is a comment on the diff rather
// than on the conversation. It is "" when there is no pull request or no
// comment to link.
func commentURL(forge configfile.Forge, pullURL string, id int64, inline bool) string {
	if pullURL == "" || id <= 0 {
		return ""
	}
	if forge == configfile.ForgeGitHub && inline {
		return fmt.Sprintf("%s#discussion_r%d", pullURL, id)
	}
	// GitHub anchors a conversation comment so. Forgejo and Gitea number
	// every comment of a pull request as an issue comment, a submitted
	// review's diff comments included, and anchor each one so on the
	// conversation.
	return fmt.Sprintf("%s#issuecomment-%d", pullURL, id)
}
