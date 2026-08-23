package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/viper"
)

// Config is the root application configuration. It is populated from explicit
// defaults, an optional config.yaml in the working directory, and environment
// variables (in increasing precedence: defaults < config.yaml < env).
type Config struct {
	Server  ServerConfig  `mapstructure:"server"`
	Data    DataConfig    `mapstructure:"data"`
	LLM     LLMConfig     `mapstructure:"llm"`
	GitHub  GitHubConfig  `mapstructure:"github"`
	Forgejo ForgejoConfig `mapstructure:"forgejo"`
	Admin   AdminConfig   `mapstructure:"admin"`
}

// ServerConfig controls the HTTP listener.
type ServerConfig struct {
	Host string `mapstructure:"host"`
	Port int    `mapstructure:"port"`
}

// DataConfig points at the directory holding the SQLite DB and git clones.
type DataConfig struct {
	Dir string `mapstructure:"dir"`
}

// LLMConfig configures the OpenAI-compatible LLM endpoint.
type LLMConfig struct {
	BaseURL       string  `mapstructure:"base_url"`
	APIKey        string  `mapstructure:"api_key"`
	Model         string  `mapstructure:"model"`
	Temperature   float64 `mapstructure:"temperature"`
	MaxTokens     int     `mapstructure:"max_tokens"`
	ThinkingLevel string  `mapstructure:"thinking_level"` // "" | "off" | "low" | "medium" | "high"; "" = no default
	TimeoutS      int     `mapstructure:"timeout_s"`
	CommitTypes   string  `mapstructure:"commit_types"`
}

// GitHubConfig configures the GitHub App used for API access and webhooks.
type GitHubConfig struct {
	AppID             int64  `mapstructure:"app_id"`
	AppPrivateKeyFile string `mapstructure:"app_private_key_file"`
	WebhookSecret     string `mapstructure:"webhook_secret"`
}

// ForgejoConfig configures the Forgejo instance API token and webhooks.
type ForgejoConfig struct {
	URL           string `mapstructure:"url"`
	Token         string `mapstructure:"token"`
	WebhookSecret string `mapstructure:"webhook_secret"`
}

// AdminConfig holds the bearer token protecting the dashboard and /api/*.
type AdminConfig struct {
	Token string `mapstructure:"token"`
}

// Load reads configuration from defaults, an optional config.yaml in the
// current working directory, and the environment. It only returns an error for
// an unreadable or malformed config.yaml; missing ADMIN_TOKEN or llm.base_url
// are surfaced as empty values (the `serve` command enforces their presence).
func Load() (*Config, error) {
	v := viper.New()
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	// Explicit defaults.
	v.SetDefault("server.port", 8080)
	v.SetDefault("server.host", "0.0.0.0")
	v.SetDefault("data.dir", "./data")
	v.SetDefault("llm.model", "qwen3.5-397b-a17b")
	v.SetDefault("llm.temperature", 0.85)
	v.SetDefault("llm.max_tokens", 4096)
	v.SetDefault("llm.timeout_s", 120)
	// No config-level default for llm.commit_types — an empty value falls
	// through to the recommended default types (engine.DefaultCommitTypes)
	// in pipeline.Resolve; only an explicit "*" keeps all commit types.

	// BindEnv makes every key resolvable from its env var even when it has no
	// default and no config.yaml entry. Without this, viper's Unmarshal path
	// (over AllSettings) silently drops env-only keys like ADMIN_TOKEN.
	for _, key := range []string{
		"server.port", "server.host", "data.dir",
		"llm.base_url", "llm.api_key", "llm.model", "llm.temperature", "llm.max_tokens", "llm.thinking_level", "llm.timeout_s", "llm.commit_types",
		"github.app_id", "github.app_private_key_file", "github.webhook_secret",
		"forgejo.url", "forgejo.token", "forgejo.webhook_secret",
		"admin.token",
	} {
		if err := v.BindEnv(key); err != nil {
			return nil, fmt.Errorf("binding env for %s: %w", key, err)
		}
	}

	// Optional config.yaml. If present it must parse, else Load fails.
	if _, err := os.Stat("config.yaml"); err == nil {
		v.SetConfigFile("config.yaml")
		if err := v.ReadInConfig(); err != nil {
			return nil, fmt.Errorf("reading config.yaml: %w", err)
		}
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("decoding config: %w", err)
	}
	return &cfg, nil
}

// GitHubEnabled reports whether GitHub is usable: a webhook secret is set, or
// a full GitHub App credential pair (AppID + private key file) is present.
func (c *Config) GitHubEnabled() bool {
	return c.GitHub.WebhookSecret != "" ||
		(c.GitHub.AppID != 0 && c.GitHub.AppPrivateKeyFile != "")
}

// ForgejoEnabled reports whether Forgejo is usable: an API token or webhook
// secret is set.
func (c *Config) ForgejoEnabled() bool {
	return c.Forgejo.Token != "" || c.Forgejo.WebhookSecret != ""
}
