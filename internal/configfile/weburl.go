package configfile

// GitHubWebURL is where github.com's pages are.
const GitHubWebURL = "https://github.com"

// WebURL is the web address of the forge the connection talks to, with no
// trailing slash: where its accounts, repositories and pull requests are
// browsed. It is "" for a forge it does not know the address of.
func (i *Connection) WebURL() string {
	switch {
	case i.Forge == ForgeGitHub:
		return GitHubWebURL
	case i.Forge.SelfHosted():
		return i.BaseURL()
	}
	return ""
}
