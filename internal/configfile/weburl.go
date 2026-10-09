package configfile

// GitHubWebURL is where github.com's pages are.
const GitHubWebURL = "https://github.com"

// WebURL is the web address of the forge the connection talks to, with no
// trailing slash: where its accounts, repositories and pull requests are
// browsed. It is "" for a forge it does not know the address of.
//
// SHIM(forge-port): the tree has only GitHub connections, so only
// github.com is known. The Forgejo and Gitea connections carry their
// instance's base url; return it here for them, trailing slash trimmed.
func (i *Connection) WebURL() string {
	if i.Forge == ForgeGitHub {
		return GitHubWebURL
	}
	return ""
}
