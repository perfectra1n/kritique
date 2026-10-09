package webapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/perfectra1n/kritique/internal/auth"
	"github.com/perfectra1n/kritique/internal/config"
	"github.com/perfectra1n/kritique/internal/configfile"
	"github.com/perfectra1n/kritique/internal/configfile/configfiletest"
)

func TestInstanceSettings(t *testing.T) {
	t.Setenv("TEST_KEY", "sk-secret")
	f := configfiletest.Load(t, `providers:
  gw: { type: openai, baseUrl: "https://kritique:hunter2@gw.example/v1", apiKey: { env: TEST_KEY } }
apps:
  acme-bot: { accounts: [acme, org-1], clientId: Iv1.acme, privateKey: { env: TEST_KEY }, webhookSecret: { env: TEST_KEY } }
`)
	env := []config.EnvVar{
		{Name: "KRITIQUE_ADDR", Value: ":9090", Set: true},
		{Name: "KRITIQUE_METRICS_ADDR", Value: ":8081"},
		{Name: "KRITIQUE_GATEWAY_URL", Value: "https://user:pass@gw.example", Set: true},
		{Name: "KRITIQUE_DATABASE_URL", Value: "set", Secret: true, Set: true},
	}
	rows := map[string]InstanceSetting{}
	for _, s := range instanceSettings(f, env) {
		rows[s.Section+" "+s.Key] = s
	}
	for key, want := range map[string]InstanceSetting{
		"environment KRITIQUE_ADDR":         {"environment", "KRITIQUE_ADDR", ":9090", configfile.SourceEnv},
		"environment KRITIQUE_METRICS_ADDR": {"environment", "KRITIQUE_METRICS_ADDR", ":8081", configfile.SourceDefault},
		"environment KRITIQUE_GATEWAY_URL": {
			"environment", "KRITIQUE_GATEWAY_URL", "https://gw.example (credentials hidden)", configfile.SourceEnv,
		},
		"environment KRITIQUE_DATABASE_URL": {"environment", "KRITIQUE_DATABASE_URL", "set", configfile.SourceEnv},
		"apps acme-bot":                     {"apps", "acme-bot", "acme, org-1, webhook /hooks/acme-bot", configfile.SourceFile},
	} {
		if rows[key] != want {
			t.Errorf("%s = %+v, want %+v", key, rows[key], want)
		}
	}
	for _, s := range rows {
		if strings.Contains(s.Value, "hunter2") || strings.Contains(s.Value, "sk-secret") || strings.Contains(s.Value, "pass@") {
			t.Fatalf("%s %s shows a secret: %q", s.Section, s.Key, s.Value)
		}
	}
}

// TestInstanceSettingsFileLayer: the providers, default models and
// embedder the file and the environment set are listed with their source,
// and without their keys.
func TestInstanceSettingsFileLayer(t *testing.T) {
	t.Setenv("TEST_KEY", "sk-secret")
	t.Setenv("KRITIQUE_REVIEW_MODEL", "gw/big")
	f, err := configfile.Parse([]byte(`providers:
  gw: { type: openai, baseUrl: "https://kritique:hunter2@gw.example/v1", apiKey: { env: TEST_KEY } }
review: { fallback: gw/small }
trigger: { settle: 45s }
embedding: { model: gw/e1, dims: 8 }
`))
	if err != nil {
		t.Fatal(err)
	}
	rows := map[string]InstanceSetting{}
	for _, s := range instanceSettings(f, nil) {
		rows[s.Section+" "+s.Key] = s
	}
	for key, want := range map[string]InstanceSetting{
		"providers gw":    {"providers", "gw", "openai at https://gw.example/v1 (credentials hidden)", configfile.SourceFile},
		"review model":    {"review", "model", "gw/big", configfile.SourceEnv},
		"review fallback": {"review", "fallback", "gw/small", configfile.SourceFile},
		"trigger settle":  {"trigger", "settle", "45s", configfile.SourceFile},
		"embedding gw/e1": {"embedding", "gw/e1", "8 dimensions", configfile.SourceFile},
	} {
		if rows[key] != want {
			t.Errorf("%s = %+v, want %+v", key, rows[key], want)
		}
	}
	for _, s := range rows {
		if strings.Contains(s.Value, "hunter2") || strings.Contains(s.Value, "sk-secret") {
			t.Fatalf("%s %s shows a secret: %q", s.Section, s.Key, s.Value)
		}
	}
}

func TestInstanceSettingsAreAdminOnly(t *testing.T) {
	ts := newTestServer(t, "https://kritique.example")
	ts.srv.env = []config.EnvVar{{Name: "KRITIQUE_ADDR", Value: ":8080"}}
	get := func(p *auth.Principal) *httptest.ResponseRecorder {
		return ts.as(p, httptest.NewRequest("GET", "/api/v1/admin/instance", nil))
	}
	if w := get(memberOf(t, ts.file, "alpha")); w.Code != http.StatusNotFound {
		t.Fatalf("account admin: status = %d, want 404", w.Code)
	}
	w := get(&auth.Principal{Admin: true})
	var rows []InstanceSetting
	if err := json.Unmarshal(w.Body.Bytes(), &rows); w.Code != http.StatusOK || err != nil || rows[0].Key != "KRITIQUE_ADDR" {
		t.Fatalf("admin: %d %s", w.Code, w.Body)
	}
}
