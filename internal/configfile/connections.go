package configfile

import (
	"cmp"
	"fmt"
	"maps"
	"net/url"
	"slices"
	"strings"
)

// GitHubApp is a GitHub App credential owned by a connection. The client
// id is not secret, but admins often keep it next to the key, so it may
// be given inline or by reference. WebhookSecret is every forge's: a
// Forgejo or Gitea connection sets it alone of the three.
type GitHubApp struct {
	ClientID      ValueOrRef `yaml:"clientId"`
	PrivateKey    SecretRef  `yaml:"privateKey"`
	WebhookSecret SecretRef  `yaml:"webhookSecret"`

	clientID      string
	privateKey    Secret
	webhookSecret Secret
}

// ClientIDValue returns the client id, inline or resolved.
func (a GitHubApp) ClientIDValue() string { return a.clientID }

// PrivateKeyValue returns the resolved private key PEM.
func (a GitHubApp) PrivateKeyValue() Secret { return a.privateKey }

// Connection is one GitHub App, or one Forgejo or Gitea bot account,
// serving the accounts it lists, an entry of the configuration's apps,
// which are keyed by name. The name is the hook path, /hooks/{name}.
type Connection struct {
	// Name is the entry's key under apps.
	Name string `yaml:"-"`
	// Forge is the forge the connection talks to, ForgeGitHub when the
	// entry names none.
	Forge Forge `yaml:"forge,omitempty"`
	// Accounts are the users and organizations the connection serves: a
	// webhook for any other account is ignored, and a repository belongs to
	// the connection serving its owner. A public GitHub App installed on
	// several organizations lists each one kritique reviews for; nothing is
	// served that is not listed.
	Accounts []string `yaml:"accounts"`

	App GitHubApp `yaml:",inline"`

	// URL is a Forgejo or Gitea instance's base URL, such as
	// https://forgejo.example.com, which its API is under at /api/v1.
	URL string `yaml:"url,omitempty"`
	// Token is the Forgejo or Gitea bot account's API token, which posts
	// the reviews: it needs write access to issues and repositories.
	Token SecretRef `yaml:"token,omitempty"`
	// GitToken is a Forgejo or Gitea token runners fetch the repository
	// with in place of Token: a runner reads untrusted content and runs
	// commands an agent chose, so a token that can only read belongs here.
	// Without one, runners are handed Token.
	GitToken SecretRef `yaml:"gitToken,omitempty"`

	token, gitToken Secret
}

// WebhookSecretValue returns the resolved webhook secret.
func (i *Connection) WebhookSecretValue() Secret { return i.App.webhookSecret }

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
