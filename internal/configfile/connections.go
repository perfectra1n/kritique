package configfile

import (
	"cmp"
	"fmt"
	"maps"
	"net/url"
	"slices"
	"strings"
)

// SelfHosted reports whether the forge is reached at a connection's URL
// with a token, rather than on github.com through a GitHub App.
func (f Forge) SelfHosted() bool { return f == ForgeForgejo || f == ForgeGitea }

// TokenValue returns a Forgejo or Gitea connection's resolved API token.
func (i *Connection) TokenValue() Secret { return i.token }

// GitTokenValue returns a Forgejo or Gitea connection's resolved fetch
// token, empty when none is configured.
func (i *Connection) GitTokenValue() Secret { return i.gitToken }

// BaseURL is a Forgejo or Gitea connection's URL without a trailing slash,
// "" for a GitHub connection.
func (i *Connection) BaseURL() string { return strings.TrimRight(i.URL, "/") }

// Host is the lowercase host, without a port, the connection's forge
// lives on: github.com, or a Forgejo or Gitea connection's URL's.
func (i *Connection) Host() string {
	if !i.Forge.SelfHosted() {
		return GitHubHost
	}
	u, err := url.Parse(i.URL)
	if err != nil {
		return ""
	}
	return strings.ToLower(u.Hostname())
}

// connectionsOf lists the file's apps in the order of their names, each
// carrying its key as its name.
func connectionsOf(apps map[string]Connection) []Connection {
	conns := make([]Connection, 0, len(apps))
	for _, name := range slices.Sorted(maps.Keys(apps)) {
		in := apps[name]
		in.Name, in.Forge = name, cmp.Or(in.Forge, ForgeGitHub)
		conns = append(conns, in)
	}
	return conns
}

// validateConnections checks one layer's connections: names that are hook
// paths; accounts listed; credentials that resolved; and every account
// served by one connection alone.
func validateConnections(conns []Connection) error {
	served := map[string]string{}
	for i := range conns {
		in := &conns[i]
		where := "apps." + in.Name
		if !nameRe.MatchString(in.Name) {
			return fmt.Errorf("configfile: %s: an app's name must be lowercase alphanumerics and hyphens, 1 to 63 characters", where)
		}
		if err := validateAccountNames(in.Accounts, where); err != nil {
			return err
		}
		if err := in.validate(where); err != nil {
			return err
		}
		for ai, a := range in.Accounts {
			key := AccountKey(in.Forge, a)
			if other, dup := served[key]; dup {
				return fmt.Errorf("configfile: %s.accounts[%d] %q is served by app %q already; an account is served by one app",
					where, ai, a, other)
			}
			served[key] = in.Name
		}
	}
	return nil
}

// connectionEnvPrefix starts every environment variable that declares the
// environment's app.
const connectionEnvPrefix = "KRITIQUE_APPS_"

// DefaultEnvConnection names the environment's app when KRITIQUE_APPS_NAME
// is unset.
const DefaultEnvConnection = "github"

// overlayConnectionEnv declares the one app the environment may: it
// replaces the file's app of its name whole, or joins them. It returns the
// app's name, "" when no KRITIQUE_APPS_* variable is set; a secret's variable
// carries the value itself. A variable that names no key is an error, so a
// typo is refused rather than ignored. KRITIQUE_APPS_FORGE picks the forge,
// GitHub unless set, and the app's validation refuses a key its forge does
// not take.
func overlayConnectionEnv(apps map[string]Connection, environ []string) (string, error) {
	in := Connection{Name: DefaultEnvConnection}
	set := false
	for _, kv := range environ {
		name, value, _ := strings.Cut(kv, "=")
		key, ok := strings.CutPrefix(name, connectionEnvPrefix)
		if !ok {
			continue
		}
		set = true
		switch key {
		case "NAME":
			in.Name = value
		case "ACCOUNTS":
			in.Accounts = envList(value)
		case "CLIENT_ID":
			in.App.ClientID = ValueOrRef{Value: value}
		case "PRIVATE_KEY":
			in.App.PrivateKey = SecretRef{Env: name}
		case "WEBHOOK_SECRET":
			in.App.WebhookSecret = SecretRef{Env: name}
		case "FORGE":
			in.Forge = Forge(value)
		case "URL":
			in.URL = value
		case "TOKEN":
			in.Token = SecretRef{Env: name}
		case "GIT_TOKEN":
			in.GitToken = SecretRef{Env: name}
		default:
			return "", fmt.Errorf("configfile: environment variable %s names no app setting", name)
		}
	}
	if !set {
		return "", nil
	}
	apps[in.Name] = in
	return in.Name, nil
}

// ConnectionFromEnv reports whether the environment declared the
// connection named name.
func (f *File) ConnectionFromEnv(name string) bool {
	return name != "" && f.envConnection == name
}
