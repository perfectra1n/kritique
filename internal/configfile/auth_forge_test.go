package configfile

import (
	"strings"
	"testing"
)

func TestForgeSignIns(t *testing.T) {
	authEnv(t)
	doc := `auth:
  admin: { password: { env: TEST_ADMIN_PASSWORD } }
  forgejo:
    url: https://git.example.com
    clientId: fj-client
    clientSecret: { env: TEST_WEBHOOK_SECRET }
    roleMappingExpr: '"admins" in orgs ? "admin" : ""'
  gitea:
    url: https://gitea.example.com/
    clientId: gt-client
    clientSecret: { env: TEST_WEBHOOK_SECRET }
` + minimal
	f := mustLoad(t, doc)
	fj, ok := f.Auth.SignInByType(SignInForgejo)
	if !ok || fj.Label() != "Forgejo" || fj.URL != "https://git.example.com" || fj.Mapping() == nil {
		t.Fatalf("forgejo sign-in = %+v, %v", fj, ok)
	}
	gt, ok := f.Auth.SignInByType(SignInGitea)
	if !ok || gt.Label() != "Gitea" || gt.Mapping() != nil {
		t.Fatalf("gitea sign-in = %+v, %v", gt, ok)
	}
	tests := []struct{ name, yaml, want string }{
		{"no url", strings.Replace(doc, "    url: https://git.example.com\n", "", 1), "auth.forgejo.url: is required"},
		{"issuer", strings.Replace(doc, "    url: https://git.example.com\n", "    url: https://git.example.com\n    issuer: https://x\n", 1),
			"auth.forgejo.issuer is for oidc"},
		{"url on oidc", authMinimal + "    url: https://x.example.com\n" + minimal, "auth.github.url is for forgejo and gitea sign-ins"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := loadBytes(t, []byte(tt.yaml)); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error %v does not mention %q", err, tt.want)
			}
		})
	}
	t.Run("env", func(t *testing.T) {
		t.Setenv("KRITIQUE_AUTH_FORGEJO_URL", "https://env.example.com")
		t.Setenv("KRITIQUE_AUTH_FORGEJO_CLIENT_ID", "env-client")
		t.Setenv("KRITIQUE_AUTH_FORGEJO_CLIENT_SECRET", "env-secret")
		t.Setenv("KRITIQUE_AUTH_GITEA_ROLE_MAPPING_EXPR", `login == "x" ? "admin" : ""`)
		f := mustLoad(t, doc)
		fj, _ := f.Auth.SignInByType(SignInForgejo)
		if fj.URL != "https://env.example.com" || fj.ClientID != "env-client" || fj.ClientSecretValue().Value() != "env-secret" ||
			!f.Auth.FromEnv("forgejo.url") || !f.Auth.FromEnv("forgejo.clientSecret") {
			t.Fatalf("forgejo sign-in from env = %+v", fj)
		}
		if gt, _ := f.Auth.SignInByType(SignInGitea); gt.Mapping() == nil || !f.Auth.FromEnv("gitea.roleMappingExpr") {
			t.Fatal("gitea mapping from env not set")
		}
	})
}
