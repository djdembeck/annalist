package config

import (
	"os"
	"path/filepath"
	"testing"
)

// chdir moves the process into dir for the duration of the test and restores
// the prior working directory. Load() resolves config.yaml relative to the
// current working directory, so these tests drive it via a temp dir.
func chdir(t *testing.T, dir string) {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(wd); err != nil {
			t.Errorf("restore wd: %v", err)
		}
	})
}

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLoadDefaults(t *testing.T) {
	chdir(t, t.TempDir())

	// The shell that runs these tests may export FORGEJO_* creds; clear them so
	// the default-config assertions are not contaminated by the host env.
	t.Setenv("FORGEJO_TOKEN", "")
	t.Setenv("FORGEJO_URL", "")
	t.Setenv("FORGEJO_WEBHOOK_SECRET", "")
	t.Setenv("ADMIN_TOKEN", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Server.Port != 8080 {
		t.Errorf("server.port = %d, want 8080", cfg.Server.Port)
	}
	if cfg.Server.Host != "0.0.0.0" {
		t.Errorf("server.host = %q, want 0.0.0.0", cfg.Server.Host)
	}
	if cfg.Data.Dir != "./data" {
		t.Errorf("data.dir = %q, want ./data", cfg.Data.Dir)
	}
	if cfg.LLM.Model != "qwen3.5-397b-a17b" {
		t.Errorf("llm.model = %q", cfg.LLM.Model)
	}
	if cfg.LLM.Temperature != 0.85 {
		t.Errorf("llm.temperature = %v, want 0.85", cfg.LLM.Temperature)
	}
	if cfg.LLM.MaxTokens != 4096 {
		t.Errorf("llm.max_tokens = %d, want 4096", cfg.LLM.MaxTokens)
	}
	if cfg.LLM.TimeoutS != 120 {
		t.Errorf("llm.timeout_s = %d, want 120", cfg.LLM.TimeoutS)
	}
	if cfg.Forgejo.URL != "" {
		t.Errorf("forgejo.url = %q, want empty (no default; set FORGEJO_URL to enable Forgejo)", cfg.Forgejo.URL)
	}
	// Env-only keys with no default must surface as empty strings.
	if cfg.Admin.Token != "" {
		t.Errorf("admin.token default = %q, want empty", cfg.Admin.Token)
	}
	if cfg.GitHub.AppID != 0 {
		t.Errorf("github.app_id default = %d, want 0", cfg.GitHub.AppID)
	}
	if cfg.GitHubEnabled() || cfg.ForgejoEnabled() {
		t.Errorf("clean default config should not be enabled; GitHub=%v Forgejo=%v",
			cfg.GitHubEnabled(), cfg.ForgejoEnabled())
	}
}

func TestLoadEnvBinding(t *testing.T) {
	chdir(t, t.TempDir())

	t.Setenv("SERVER_PORT", "9000")
	t.Setenv("DATA_DIR", "/tmp/envdata")
	t.Setenv("ADMIN_TOKEN", "sekret")
	t.Setenv("GITHUB_APP_ID", "42")
	t.Setenv("GITHUB_APP_PRIVATE_KEY_FILE", "/keys/app.pem")
	t.Setenv("GITHUB_WEBHOOK_SECRET", "gh-secret")
	t.Setenv("FORGEJO_URL", "https://forgejo.example.com")
	t.Setenv("FORGEJO_TOKEN", "forgejo-token")
	t.Setenv("LLM_BASE_URL", "https://llm.example.com/v1")
	t.Setenv("LLM_API_KEY", "llm-key")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Server.Port != 9000 {
		t.Errorf("server.port = %d, want 9000", cfg.Server.Port)
	}
	if cfg.Data.Dir != "/tmp/envdata" {
		t.Errorf("data.dir = %q", cfg.Data.Dir)
	}
	if cfg.Admin.Token != "sekret" {
		t.Errorf("admin.token = %q", cfg.Admin.Token)
	}
	if cfg.GitHub.AppID != 42 {
		t.Errorf("github.app_id = %d, want 42", cfg.GitHub.AppID)
	}
	if cfg.GitHub.AppPrivateKeyFile != "/keys/app.pem" {
		t.Errorf("github.app_private_key_file = %q", cfg.GitHub.AppPrivateKeyFile)
	}
	if cfg.GitHub.WebhookSecret != "gh-secret" {
		t.Errorf("github.webhook_secret = %q", cfg.GitHub.WebhookSecret)
	}
	if cfg.Forgejo.URL != "https://forgejo.example.com" {
		t.Errorf("forgejo.url = %q", cfg.Forgejo.URL)
	}
	if cfg.Forgejo.Token != "forgejo-token" {
		t.Errorf("forgejo.token = %q", cfg.Forgejo.Token)
	}
	if cfg.LLM.BaseURL != "https://llm.example.com/v1" || cfg.LLM.APIKey != "llm-key" {
		t.Errorf("llm base_url/api_key = %q/%q", cfg.LLM.BaseURL, cfg.LLM.APIKey)
	}
	if !cfg.GitHubEnabled() {
		t.Error("GitHubEnabled() should be true with full GitHub creds")
	}
	if !cfg.ForgejoEnabled() {
		t.Error("ForgejoEnabled() should be true with a token set")
	}
}

func TestLoadConfigFilePrecedence(t *testing.T) {
	// Defaults(0.85) < config.yaml(0.2) < env(0.7).
	dir := t.TempDir()
	writeFile(t, dir, "config.yaml", `
server:
  port: 9090
llm:
  temperature: 0.2
  model: from-file-model
admin:
  token: file-token
`)
	chdir(t, dir)

	t.Setenv("LLM_TEMPERATURE", "0.7")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Server.Port != 9090 {
		t.Errorf("server.port = %d, want 9090 (from config.yaml)", cfg.Server.Port)
	}
	if cfg.LLM.Model != "from-file-model" {
		t.Errorf("llm.model = %q, want from-file-model", cfg.LLM.Model)
	}
	if cfg.Admin.Token != "file-token" {
		t.Errorf("admin.token = %q, want file-token", cfg.Admin.Token)
	}
	// env must beat config.yaml.
	if cfg.LLM.Temperature != 0.7 {
		t.Errorf("llm.temperature = %v, want 0.7 (env beats config.yaml)", cfg.LLM.Temperature)
	}
}

func TestLoadMalformedConfigFile(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "config.yaml", "server: [unclosed")
	chdir(t, dir)

	if _, err := Load(); err == nil {
		t.Fatal("Load() should error on a malformed config.yaml")
	}
}

func TestGitHubEnabled(t *testing.T) {
	cases := []struct {
		name string
		cfg  GitHubConfig
		want bool
	}{
		{name: "empty", cfg: GitHubConfig{}, want: false},
		{name: "app id only", cfg: GitHubConfig{AppID: 7}, want: false},
		{name: "key file only", cfg: GitHubConfig{AppPrivateKeyFile: "/x.pem"}, want: false},
		{name: "webhook secret only", cfg: GitHubConfig{WebhookSecret: "secret"}, want: true},
		{name: "full app pair", cfg: GitHubConfig{AppID: 7, AppPrivateKeyFile: "/x.pem"}, want: true},
		{name: "full pair plus secret", cfg: GitHubConfig{AppID: 7, AppPrivateKeyFile: "/x.pem", WebhookSecret: "s"}, want: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := &Config{GitHub: tc.cfg}
			if got := c.GitHubEnabled(); got != tc.want {
				t.Errorf("GitHubEnabled() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestForgejoEnabled(t *testing.T) {
	cases := []struct {
		name string
		cfg  ForgejoConfig
		want bool
	}{
		{name: "empty", cfg: ForgejoConfig{}, want: false},
		{name: "url only", cfg: ForgejoConfig{URL: "https://x"}, want: false},
		{name: "token only", cfg: ForgejoConfig{Token: "tok"}, want: true},
		{name: "webhook secret only", cfg: ForgejoConfig{WebhookSecret: "s"}, want: true},
		{name: "both token and secret", cfg: ForgejoConfig{Token: "t", WebhookSecret: "s"}, want: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := &Config{Forgejo: tc.cfg}
			if got := c.ForgejoEnabled(); got != tc.want {
				t.Errorf("ForgejoEnabled() = %v, want %v", got, tc.want)
			}
		})
	}
}
