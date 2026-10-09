package configfile

import (
	"slices"
	"strings"
	"testing"
)

// forgejoApps is a Forgejo app and a Gitea one beside minimal's GitHub app,
// with an entry for an account each serves.
var forgejoApps = minimal + `  home-forgejo:
    forge: forgejo
    url: https://git.example.com/
    accounts: [Perfectra1n, acme]
    token: { env: TEST_FORGE_TOKEN }
    gitToken: { env: TEST_FORGE_GIT_TOKEN }
    webhookSecret: { env: TEST_WEBHOOK_SECRET }
  work-gitea:
    forge: gitea
    url: http://gitea.internal:3000/sub
    accounts: [team]
    token: { env: TEST_FORGE_TOKEN }
    webhookSecret: { env: TEST_WEBHOOK_SECRET }
accounts:
  perfectra1n: { limits: { reviewsPerDay: 7 } }
`

func forgeEnv(t *testing.T) {
	t.Helper()
	t.Setenv("TEST_PRIVATE_KEY", "key")
	t.Setenv("TEST_WEBHOOK_SECRET", "whsec")
	t.Setenv("TEST_FORGE_TOKEN", "fj-token\n")
	t.Setenv("TEST_FORGE_GIT_TOKEN", "fj-read")
	t.Setenv("TEST_EMPTY", "")
}

func TestForgejoConnections(t *testing.T) {
	forgeEnv(t)
	f := mustLoad(t, forgejoApps)
	fj, ok := f.Connection("home-forgejo")
	if !ok || fj.Forge != ForgeForgejo || fj.BaseURL() != "https://git.example.com" || fj.Host() != "git.example.com" {
		t.Fatalf("home-forgejo = %+v, %v", fj, ok)
	}
	if fj.TokenValue().Value() != "fj-token" || fj.GitTokenValue().Value() != "fj-read" || fj.WebhookSecretValue().Value() != "whsec" {
		t.Fatal("forgejo credentials not resolved")
	}
	gt, _ := f.Connection("work-gitea")
	if gt.Forge != ForgeGitea || gt.Host() != "gitea.internal" || gt.GitTokenValue().Value() != "" {
		t.Fatalf("work-gitea = %+v", gt)
	}
	gh, _ := f.Connection("acme-bot")
	if gh.Forge != ForgeGitHub || gh.Host() != GitHubHost {
		t.Fatalf("acme-bot = %+v", gh)
	}
	// The same name on two forges is two accounts, and an entry keyed by
	// the name alone applies to the account of that name on each forge.
	tests := []struct {
		forge  Forge
		name   string
		key    string
		perDay int
	}{
		{ForgeGitHub, "acme", "github/acme", 0},
		{ForgeForgejo, "acme", "forgejo/acme", 0},
		{ForgeForgejo, "perfectra1n", "forgejo/perfectra1n", 7},
		{ForgeGitea, "team", "gitea/team", 0},
	}
	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			a, ok := f.Account(tt.forge, tt.name)
			if !ok || a.Key() != tt.key || a.Forge != tt.forge {
				t.Fatalf("Account(%s, %s) = %+v, %v", tt.forge, tt.name, a, ok)
			}
			if c := f.ConnectionFor(a); c == nil || c.Forge != tt.forge {
				t.Fatalf("ConnectionFor(%s) = %+v", tt.key, c)
			}
			got := 0
			if a.Limits.ReviewsPerDay != nil {
				got = *a.Limits.ReviewsPerDay
			}
			if got != tt.perDay {
				t.Fatalf("%s reviewsPerDay = %d, want %d", tt.key, got, tt.perDay)
			}
		})
	}
	if _, ok := f.Account(ForgeGitHub, "perfectra1n"); ok {
		t.Fatal("an account no github app serves must not run")
	}
	if len(f.Unserved()) != 0 {
		t.Fatalf("Unserved = %v", f.Unserved())
	}
}

// TestAccountIDsMatchStoredRows pins the forge strings and the account id
// they derive: the database keys every Forgejo and Gitea row by them.
func TestAccountIDsMatchStoredRows(t *testing.T) {
	for _, tt := range []struct {
		forge Forge
		want  string
	}{{ForgeForgejo, "forgejo"}, {ForgeGitea, "gitea"}, {ForgeGitHub, "github"}} {
		if string(tt.forge) != tt.want {
			t.Fatalf("forge %q must stay %q", tt.forge, tt.want)
		}
		if AccountKey(tt.forge, "Perfectra1n") != tt.want+"/perfectra1n" {
			t.Fatalf("AccountKey = %q", AccountKey(tt.forge, "Perfectra1n"))
		}
		a := Account{Forge: tt.forge, Name: "PERFECTRA1N"}
		if a.ID() != AccountID(tt.forge, "perfectra1n") {
			t.Fatal("an account's id must not depend on the name's case")
		}
	}
	if AccountID(ForgeForgejo, "x") == AccountID(ForgeGitea, "x") {
		t.Fatal("forgejo and gitea accounts must not share an id")
	}
}

func TestForgejoConnectionRejects(t *testing.T) {
	forgeEnv(t)
	app := func(fields string) string {
		return minimal + "  home: { forge: forgejo, accounts: [me], webhookSecret: { env: TEST_WEBHOOK_SECRET }, " + fields + " }\n"
	}
	ok := "url: https://git.example.com, token: { env: TEST_FORGE_TOKEN }"
	if _, err := loadBytes(t, []byte(app(ok))); err != nil {
		t.Fatalf("a forgejo app must parse: %v", err)
	}
	tests := []struct {
		name, yaml, want string
	}{
		{"no url", app("token: { env: TEST_FORGE_TOKEN }"), "apps.home.url: is required"},
		{"bare host", app("url: git.example.com, token: { env: TEST_FORGE_TOKEN }"), "apps.home.url: \"git.example.com\" must be an http or https URL"},
		{"ssh url", app("url: ssh://git.example.com, token: { env: TEST_FORGE_TOKEN }"), "must be an http or https URL"},
		{"url with a query", app("url: https://git.example.com/?x=1, token: { env: TEST_FORGE_TOKEN }"), "must not carry credentials, a query"},
		{"url with credentials", app("url: https://u:p@git.example.com, token: { env: TEST_FORGE_TOKEN }"), "must not carry credentials"},
		{"no token", app("url: https://git.example.com"), "apps.home.token: reference must set env"},
		{"empty token", app("url: https://git.example.com, token: { env: TEST_EMPTY }"), "apps.home.token is required"},
		{"empty git token", app(ok + ", gitToken: { env: TEST_EMPTY }"), "apps.home.gitToken resolved to an empty value"},
		{"a client id", app(ok + ", clientId: Iv1.x"), "a forgejo app takes url, token and gitToken, not clientId or privateKey"},
		{"a private key", app(ok + ", privateKey: { env: TEST_PRIVATE_KEY }"), "not clientId or privateKey"},
		{"no webhook secret", strings.Replace(app(ok), "webhookSecret: { env: TEST_WEBHOOK_SECRET }, ", "", 1), "apps.home.webhookSecret: reference must set env"},
		{"a url on github", strings.Replace(minimal, "acme-bot: { ", "acme-bot: { url: https://github.com, ", 1),
			"apps.acme-bot.url is for a forgejo or gitea app"},
		{"a token on github", strings.Replace(minimal, "acme-bot: { ", "acme-bot: { token: { env: TEST_FORGE_TOKEN }, ", 1),
			"apps.acme-bot.token is for a forgejo or gitea app"},
		{"one account served twice on a forge", app(ok) + "  home2: { forge: forgejo, accounts: [ME], url: https://other.example.com, " +
			"token: { env: TEST_FORGE_TOKEN }, webhookSecret: { env: TEST_WEBHOOK_SECRET } }\n", "is served by app \"home\" already"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := loadBytes(t, []byte(tt.yaml))
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error %v does not mention %q", err, tt.want)
			}
		})
	}
}

// TestForgejoConnectionEnv: the KRITIQUE_APPS_* variables declare a Forgejo
// or Gitea app too, and refuse a GitHub key on one.
func TestForgejoConnectionEnv(t *testing.T) {
	forgeEnv(t)
	t.Setenv("KRITIQUE_APPS_NAME", "env-forgejo")
	t.Setenv("KRITIQUE_APPS_FORGE", "gitea")
	t.Setenv("KRITIQUE_APPS_URL", "https://gitea.example.com")
	t.Setenv("KRITIQUE_APPS_ACCOUNTS", "org-1, user-1")
	t.Setenv("KRITIQUE_APPS_TOKEN", "tok\n")
	t.Setenv("KRITIQUE_APPS_GIT_TOKEN", "read")
	t.Setenv("KRITIQUE_APPS_WEBHOOK_SECRET", "hook")
	f := mustLoad(t, minimal)
	in, ok := f.Connection("env-forgejo")
	if !ok || !f.ConnectionFromEnv("env-forgejo") || in.Forge != ForgeGitea || in.BaseURL() != "https://gitea.example.com" {
		t.Fatalf("env connection = %+v, %v", in, ok)
	}
	if in.TokenValue().Value() != "tok" || in.GitTokenValue().Value() != "read" || in.WebhookSecretValue().Value() != "hook" {
		t.Fatal("env connection's secrets not resolved")
	}
	if !slices.Equal(in.Accounts, []string{"org-1", "user-1"}) {
		t.Fatalf("accounts = %v", in.Accounts)
	}
	for _, name := range []string{"KRITIQUE_APPS_TOKEN", "KRITIQUE_APPS_GIT_TOKEN", "KRITIQUE_APPS_WEBHOOK_SECRET"} {
		if !slices.Contains(f.SecretEnv(), name) {
			t.Fatalf("SecretEnv = %v, missing %s", f.SecretEnv(), name)
		}
	}
	t.Setenv("KRITIQUE_APPS_PRIVATE_KEY", "pem")
	if _, err := Parse([]byte(minimal)); err == nil || !strings.Contains(err.Error(), "a gitea app takes url, token and gitToken") {
		t.Fatalf("a private key on a gitea app: %v", err)
	}
}

func TestForgeEgressAndCommands(t *testing.T) {
	forgeEnv(t)
	f := mustLoad(t, forgejoApps+"agent: { commands: [gh, rg] }\n")
	rules := f.EgressRules()
	for host, want := range map[string]bool{
		"github.com": true, "api.github.com": true, "git.example.com": true, "gitea.internal": true, "example.com": false,
	} {
		if rules.Allows(host) != want {
			t.Errorf("Allows(%s) = %v, want %v", host, !want, want)
		}
	}
	if _, err := loadBytes(t, []byte(forgejoApps+"egress: { deny: [git.example.com] }\n")); err == nil ||
		!strings.Contains(err.Error(), "git.example.com is denied, which an app needs") {
		t.Fatalf("denying a forge's host: %v", err)
	}
	gh, _ := f.Account(ForgeGitHub, "acme")
	fj, _ := f.Account(ForgeForgejo, "acme")
	if got := f.Settings(gh, "acme/x").Agent.Commands; !slices.Equal(got, []string{"gh", "rg"}) {
		t.Fatalf("github commands = %v", got)
	}
	if got := f.Settings(fj, "acme/x").Agent.Commands; !slices.Equal(got, []string{"rg"}) {
		t.Fatalf("forgejo commands = %v, want gh left out", got)
	}
	if got := f.Settings(gh, "acme/x").Agent.Commands; !slices.Equal(got, []string{"gh", "rg"}) {
		t.Fatal("leaving gh out for one account changed another's")
	}
}
