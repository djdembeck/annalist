package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"runtime"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"golang.org/x/sync/errgroup"

	"github.com/djdembeck/annalist/internal/db"
	"github.com/djdembeck/annalist/internal/engine"
	"github.com/djdembeck/annalist/internal/llm"
	"github.com/djdembeck/annalist/internal/pipeline"
	"github.com/djdembeck/annalist/internal/version"
)

// In-repo instruction file paths per platform.
const (
	instructionsPathGitHub  = ".github/release-notes-instructions.md"
	instructionsPathForgejo = ".forgejo/release-notes.md"
)

// Source labels for effective value origin.
const (
	sourceGlobal = "global"
	sourceRepo   = "repo"
)

func validPlatform(p string) bool {
	return p == "github" || p == "forgejo"
}

// validMode reports whether m is an accepted generation mode: "" (inherit /
// use the resolved default), engine.ModeLite, or engine.ModeDeep.
func validMode(m string) bool {
	return m == "" || m == engine.ModeLite || m == engine.ModeDeep
}

// thinkingLevelValues are the accepted thinking_level values for the settings
// PUTs: "" (inherit) or one of the named levels, including "off" which
// explicitly disables extended thinking even when config/env set a level.
var thinkingLevelValues = []string{"", "off", "low", "medium", "high"}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func badJSON(w http.ResponseWriter, err error) {
	writeErr(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
}

// setPresenceInt handles an optional integer field in a presence map: an
// explicit null clears *target to 0 (inherit), a present value sets it after
// checking it is >= min. A missing key leaves *target untouched.
func setPresenceInt(m map[string]json.RawMessage, key string, target *int, min int) error {
	raw, ok := m[key]
	if !ok {
		return nil
	}
	if string(raw) == "null" {
		*target = 0
		return nil
	}
	var v int
	if err := json.Unmarshal(raw, &v); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}
	if v < min {
		return fmt.Errorf("%s must be >= %d", key, min)
	}
	*target = v
	return nil
}

// setPresenceStr handles an optional string field in a presence map: an
// explicit null clears *target to "", a present value sets it after checking
// it is one of allowed. A missing key leaves *target untouched.
func setPresenceStr(m map[string]json.RawMessage, key string, target *string, allowed ...string) error {
	raw, ok := m[key]
	if !ok {
		return nil
	}
	if string(raw) == "null" {
		*target = ""
		return nil
	}
	var v string
	if err := json.Unmarshal(raw, &v); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}
	for _, a := range allowed {
		if v == a {
			*target = v
			return nil
		}
	}
	return fmt.Errorf("invalid %s", key)
}

// decodePresenceMap reads the request body as an object while preserving which
// keys were present, so absent fields (keep) can be distinguished from explicit
// nulls (clear).
func decodePresenceMap(r *http.Request) (map[string]json.RawMessage, error) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, err
	}
	body = bytes.TrimSpace(body)
	if len(body) == 0 {
		body = []byte("{}")
	}
	m := map[string]json.RawMessage{}
	if err := json.Unmarshal(body, &m); err != nil {
		return nil, err
	}
	return m, nil
}

// effective is the resolved (inherited) tone/model/temperature/instructions preview.
type effective struct {
	Tone          string   `json:"tone"`
	Model         string   `json:"model"`
	Temperature   float64  `json:"temperature"`
	Instructions  string   `json:"instructions"`
	Source        string   `json:"source"`
	CommitTypes   []string `json:"commit_types"`
	Mode          string   `json:"mode"`
	MaxTokens     int      `json:"max_tokens"`
	ThinkingLevel string   `json:"thinking_level"`
}

// repoItemResp is the JSON shape for a repo in /api/repos and the settings PUT.
type repoItemResp struct {
	Platform      string    `json:"platform"`
	Owner         string    `json:"owner"`
	Repo          string    `json:"repo"`
	Enabled       bool      `json:"enabled"`
	Tone          string    `json:"tone"`
	Instructions  string    `json:"instructions"`
	Model         string    `json:"model"`
	Temperature   *float64  `json:"temperature"`
	Trigger       string    `json:"trigger"`
	CommitTypes   string    `json:"commit_types"`
	Mode          string    `json:"mode"`
	MaxTokens     int       `json:"max_tokens"`
	ThinkingLevel string    `json:"thinking_level"`
	Effective     effective `json:"effective"`
}

// repoItem builds the response for a repo_settings row, computing the
// effective (inherited) values via the pipeline's Resolve.
func (a *api) repoItem(ctx context.Context, row db.RepoSetting) (repoItemResp, error) {
	_, _, resolved, err := a.pip.Resolve(ctx, row.Platform, row.Owner, row.Repo)
	if err != nil {
		return repoItemResp{}, err
	}

	source := sourceGlobal
	if row.Instructions != "" {
		source = sourceRepo
	}

	return repoItemResp{
		Platform:      row.Platform,
		Owner:         row.Owner,
		Repo:          row.Repo,
		Enabled:       row.Enabled,
		Tone:          row.Tone,
		Instructions:  row.Instructions,
		Model:         row.Model,
		Temperature:   row.Temperature,
		Trigger:       row.Trigger,
		CommitTypes:   row.CommitTypes,
		Mode:          row.Mode,
		MaxTokens:     row.MaxTokens,
		ThinkingLevel: row.ThinkingLevel,
		Effective: effective{
			Tone:          resolved.Tone,
			Model:         resolved.Model,
			Temperature:   resolved.Temperature,
			Instructions:  resolved.Instructions,
			Source:        source,
			CommitTypes:   resolved.CommitTypes,
			Mode:          resolved.Mode,
			MaxTokens:     resolved.MaxTokens,
			ThinkingLevel: resolved.ThinkingLevel,
		},
	}, nil
}

// settingsFor returns the stored row for a repo, or a default row when absent.
func (a *api) settingsFor(ctx context.Context, platform, owner, repo string) (db.RepoSetting, error) {
	row, err := a.store.GetRepoSettings(platform, owner, repo)
	if err != nil {
		return db.RepoSetting{}, err
	}
	setting := db.RepoSetting{Platform: platform, Owner: owner, Repo: repo, Enabled: true, Trigger: "auto"}
	if row != nil {
		setting = *row
	}
	return setting, nil
}

// handleHealth is the open (no-auth) liveness probe.
func (a *api) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "version": version.Version})
}

// handleStatus reports which platforms are enabled.
func (a *api) handleStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"github":  a.cfg.GitHubEnabled(),
		"forgejo": a.cfg.ForgejoEnabled(),
		"admin":   true,
	})
}

// handleListRepos returns the managed repos (rows from repo_settings), each
// with effective (inherited) values resolved via the pipeline. Platform
// discovery is intentionally not consulted here; /api/repos/available covers
// that.
func (a *api) handleListRepos(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rows, err := a.store.ListRepoSettings()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}

	items := make([]repoItemResp, len(rows))
	eg, egCtx := errgroup.WithContext(ctx)
	sem := make(chan struct{}, runtime.NumCPU())
	for i, row := range rows {
		i := i
		row := row
		eg.Go(func() error {
			sem <- struct{}{}
			defer func() { <-sem }()
			item, err := a.repoItem(egCtx, row)
			if err != nil {
				return err
			}
			items[i] = item
			return nil
		})
	}
	if err := eg.Wait(); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, items)
}

// availableRepo is the flat JSON shape for a repo in /api/repos/available.
type availableRepo struct {
	Platform     string    `json:"platform"`
	Owner        string    `json:"owner"`
	Repo         string    `json:"repo"`
	Fork         bool      `json:"fork"`
	OwnNamespace bool      `json:"ownNamespace"`
	UpdatedAt    time.Time `json:"updatedAt"`
	PushedAt     time.Time `json:"pushedAt"`
}

// repoLister is the subset shared by both platform clients that hands back the
// discoverable repositories.
type repoLister interface {
	ListRepos(ctx context.Context) ([]pipeline.OwnerRepo, error)
}

// listAvailable appends to available the repos returned by a platform client
// that are not yet managed (absent from repo_settings).
func (a *api) listAvailable(ctx context.Context, platform string, client repoLister, managed map[string]bool, available []availableRepo) ([]availableRepo, error) {
	repos, err := client.ListRepos(ctx)
	if err != nil {
		return available, fmt.Errorf("%s: %w", platform, err)
	}
	for _, rp := range repos {
		key := platform + "/" + rp.Owner + "/" + rp.Repo
		if managed[key] {
			continue
		}
		available = append(available, availableRepo{Platform: platform, Owner: rp.Owner, Repo: rp.Repo, Fork: rp.Fork, OwnNamespace: rp.OwnNamespace, UpdatedAt: rp.UpdatedAt, PushedAt: rp.PushedAt})
	}
	return available, nil
}

// handleListAvailableRepos returns the repos available to add from enabled
// platforms, excluding any already present in repo_settings.
func (a *api) handleListAvailableRepos(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	managed, err := a.store.ListRepoSettings()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	managedSet := make(map[string]bool, len(managed))
	for _, row := range managed {
		managedSet[row.Platform+"/"+row.Owner+"/"+row.Repo] = true
	}

	var available []availableRepo
	var errors []string
	var enabledPlatforms int
	if a.cfg.GitHubEnabled() && a.gh != nil {
		enabledPlatforms++
		var err error
		available, err = a.listAvailable(ctx, "github", a.gh, managedSet, available)
		if err != nil {
			errors = append(errors, fmt.Sprintf("github: %s", err.Error()))
		}
	}
	if a.cfg.ForgejoEnabled() && a.fj != nil {
		enabledPlatforms++
		var err error
		available, err = a.listAvailable(ctx, "forgejo", a.fj, managedSet, available)
		if err != nil {
			errors = append(errors, fmt.Sprintf("forgejo: %s", err.Error()))
		}
	}
	if enabledPlatforms > 0 && len(errors) == enabledPlatforms {
		writeErr(w, http.StatusInternalServerError,
			"all platforms failed: "+strings.Join(errors, "; "))
		return
	}

	if available == nil {
		available = []availableRepo{}
	}
	writeJSON(w, http.StatusOK, available)
}

// handleAddRepo creates (or activates) a managed repo in repo_settings.
func (a *api) handleAddRepo(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Platform string `json:"platform"`
		Owner    string `json:"owner"`
		Repo     string `json:"repo"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		badJSON(w, err)
		return
	}
	if !validPlatform(req.Platform) {
		writeErr(w, http.StatusBadRequest, "invalid platform")
		return
	}
	if req.Owner == "" || req.Repo == "" {
		writeErr(w, http.StatusBadRequest, "owner and repo are required")
		return
	}

	setting, err := a.settingsFor(r.Context(), req.Platform, req.Owner, req.Repo)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Preserve existing per-repo overrides; only set defaults for a fresh repo.
	if setting.Tone == "" {
		setting.Tone = "auto"
	}
	if setting.Instructions == "" {
		// inherits from global
	}
	if setting.Model == "" {
		// inherits from global
	}
	// Temperature 0 is valid (inherits from global), so we don't reset it.
	setting.Enabled = true
	setting.Trigger = "auto"

	if err := a.store.UpsertRepoSettings(setting); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}

	item, err := a.repoItem(r.Context(), setting)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, item)
}

// handlePutRepoSettings merges a partial settings update into the repo row.
func (a *api) handlePutRepoSettings(w http.ResponseWriter, r *http.Request) {
	platform := chi.URLParam(r, "platform")
	owner := chi.URLParam(r, "owner")
	repo := chi.URLParam(r, "repo")
	if !validPlatform(platform) {
		writeErr(w, http.StatusBadRequest, "invalid platform")
		return
	}

	m, err := decodePresenceMap(r)
	if err != nil {
		badJSON(w, err)
		return
	}

	setting, err := a.settingsFor(r.Context(), platform, owner, repo)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}

	// tone/instructions/model: present non-null sets, explicit null clears.
	for _, key := range []string{"tone", "instructions", "model"} {
		if raw, ok := m[key]; ok {
			if string(raw) == "null" {
				switch key {
				case "tone":
					setting.Tone = ""
				case "instructions":
					setting.Instructions = ""
				case "model":
					setting.Model = ""
				}
				continue
			}
			var s string
			if err := json.Unmarshal(raw, &s); err != nil {
				badJSON(w, err)
				return
			}
			switch key {
			case "tone":
				setting.Tone = s
			case "instructions":
				setting.Instructions = s
			case "model":
				setting.Model = s
			}
		}
	}

	// temperature: present value sets, explicit null clears.
	if raw, ok := m["temperature"]; ok {
		if string(raw) == "null" {
			setting.Temperature = nil
		} else {
			var v float64
			if err := json.Unmarshal(raw, &v); err != nil {
				badJSON(w, err)
				return
			}
			setting.Temperature = &v
		}
	}

	// commit_types: present non-null sets, explicit null clears.
	if raw, ok := m["commit_types"]; ok {
		if string(raw) == "null" {
			setting.CommitTypes = ""
		} else {
			var v string
			if err := json.Unmarshal(raw, &v); err != nil {
				badJSON(w, err)
				return
			}
			setting.CommitTypes = strings.Join(engine.ParseCommitTypes(v), ",")
		}
	}

	// mode: present non-null sets (must be lite|deep), explicit null clears to inherit.
	if raw, ok := m["mode"]; ok {
		if string(raw) == "null" {
			setting.Mode = ""
		} else {
			var v string
			if err := json.Unmarshal(raw, &v); err != nil {
				badJSON(w, err)
				return
			}
			if !validMode(v) {
				writeErr(w, http.StatusBadRequest, "invalid mode")
				return
			}
			setting.Mode = v
		}
	}

	// max_tokens: present value sets, explicit null clears to 0 (inherit);
	// integer >= 1 required.
	if err := setPresenceInt(m, "max_tokens", &setting.MaxTokens, 1); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}

	// thinking_level: present non-null sets, explicit null clears to ""
	// (inherit); "" | off | low | medium | high accepted.
	if err := setPresenceStr(m, "thinking_level", &setting.ThinkingLevel, thinkingLevelValues...); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}

	// enabled / trigger: applied only when present and non-null.
	if raw, ok := m["enabled"]; ok && string(raw) != "null" {
		var v bool
		if err := json.Unmarshal(raw, &v); err != nil {
			badJSON(w, err)
			return
		}
		setting.Enabled = v
	}
	if raw, ok := m["trigger"]; ok && string(raw) != "null" {
		var v string
		if err := json.Unmarshal(raw, &v); err != nil {
			badJSON(w, err)
			return
		}
		setting.Trigger = v
	}

	if err := a.store.UpsertRepoSettings(setting); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}

	item, err := a.repoItem(r.Context(), setting)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, item)
}

// handleGenerate runs the pipeline and publishes notes.
func (a *api) handleGenerate(w http.ResponseWriter, r *http.Request) {
	platform := chi.URLParam(r, "platform")
	owner := chi.URLParam(r, "owner")
	repo := chi.URLParam(r, "repo")
	if !validPlatform(platform) {
		writeErr(w, http.StatusBadRequest, "invalid platform")
		return
	}

	var req struct {
		ToTag   string  `json:"to_tag"`
		FromTag *string `json:"from_tag"`
		Force   bool    `json:"force"`
		Publish *bool   `json:"publish"`
		Mode    string  `json:"mode"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		badJSON(w, err)
		return
	}
	if strings.TrimSpace(req.ToTag) == "" {
		writeErr(w, http.StatusBadRequest, "to_tag is required")
		return
	}
	if !validMode(req.Mode) {
		writeErr(w, http.StatusBadRequest, "invalid mode")
		return
	}

	publish := false
	if req.Publish != nil {
		publish = *req.Publish
	}

	releaseID := "manual:" + platform + "/" + owner + "/" + repo + "@" + req.ToTag
	var fromTag string
	if req.FromTag != nil {
		fromTag = *req.FromTag
	}

	notes, err := a.pip.GenerateNotes(r.Context(), pipeline.Spec{
		Platform:  platform,
		Owner:     owner,
		Repo:      repo,
		ToTag:     req.ToTag,
		FromTag:   fromTag,
		ReleaseID: releaseID,
	}, pipeline.Options{Force: req.Force, Publish: publish, Mode: req.Mode})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"notes":      notes,
		"release_id": releaseID,
		"published":  publish,
	})
}

// handleInRepoInstructions reads the instructions file from the repo's
// in-repo location (.github/ or .forgejo/). It is an on-demand endpoint — the
// batch /api/repos endpoint does not call the platform API for speed.
func (a *api) handleInRepoInstructions(w http.ResponseWriter, r *http.Request) {
	platform := chi.URLParam(r, "platform")
	owner := chi.URLParam(r, "owner")
	repo := chi.URLParam(r, "repo")
	if !validPlatform(platform) {
		writeErr(w, http.StatusBadRequest, "invalid platform")
		return
	}

	instPath := instructionsPathGitHub
	if platform == "forgejo" {
		instPath = instructionsPathForgejo
	}

	var content string
	var readErr error
	if platform == "forgejo" {
		if a.fj == nil {
			writeErr(w, http.StatusServiceUnavailable, "forgejo client not configured")
			return
		}
		content, readErr = a.fj.ReadRepoFile(r.Context(), owner, repo, instPath)
	} else {
		if a.gh == nil {
			writeErr(w, http.StatusServiceUnavailable, "github client not configured")
			return
		}
		content, readErr = a.gh.ReadRepoFile(r.Context(), owner, repo, instPath)
	}

	if readErr != nil {
		if errors.Is(readErr, pipeline.ErrNotFound) {
			writeJSON(w, http.StatusOK, map[string]any{})
			return
		}
		writeErr(w, http.StatusBadGateway, readErr.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"instructions": content})
}

// effectiveLLM returns the endpoint to use: saved settings override env/config.
func (a *api) effectiveLLM(s db.Settings) (baseURL, apiKey string) {
	baseURL, apiKey = a.cfg.LLM.BaseURL, a.cfg.LLM.APIKey
	if s.BaseURL != "" {
		baseURL = s.BaseURL
	}
	if s.APIKey != "" {
		apiKey = s.APIKey
	}
	return baseURL, apiKey
}

// settingsResp is the global-settings response, including the LLM/platform block.
// The llm block reports the effective base URL (saved overrides env) and a masked
// api key indicator; the plaintext key is never returned.
func (a *api) settingsResp(s db.Settings) map[string]any {
	baseURL, _ := a.effectiveLLM(s)
	hasKey := s.APIKey != "" || a.cfg.LLM.APIKey != ""
	apiKeyPlaceholder := ""
	if hasKey {
		apiKeyPlaceholder = "••••••••"
	}
	return map[string]any{
		"tone":           s.Tone,
		"instructions":   s.Instructions,
		"model":          s.Model,
		"temperature":    s.Temperature,
		"commit_types":   s.CommitTypes,
		"mode":           s.Mode,
		"max_tokens":     s.MaxTokens,
		"thinking_level": s.ThinkingLevel,
		"llm": map[string]any{
			"base_url": baseURL,
			"api_key":  apiKeyPlaceholder,
			"has_key":  hasKey,
		},
		"github":  a.cfg.GitHubEnabled(),
		"forgejo": a.cfg.ForgejoEnabled(),
	}
}

// handleGetSettings returns the global settings row plus platform status.
func (a *api) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	s, err := a.store.GetSettings()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, a.settingsResp(s))
}

// handlePutSettings merges a partial global-settings update into the singleton row.
func (a *api) handlePutSettings(w http.ResponseWriter, r *http.Request) {
	m, err := decodePresenceMap(r)
	if err != nil {
		badJSON(w, err)
		return
	}

	s, err := a.store.GetSettings()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}

	for _, key := range []string{"tone", "instructions", "model"} {
		if raw, ok := m[key]; ok {
			if string(raw) == "null" {
				switch key {
				case "tone":
					s.Tone = ""
				case "instructions":
					s.Instructions = ""
				case "model":
					s.Model = ""
				}
				continue
			}
			var v string
			if err := json.Unmarshal(raw, &v); err != nil {
				badJSON(w, err)
				return
			}
			switch key {
			case "tone":
				s.Tone = v
			case "instructions":
				s.Instructions = v
			case "model":
				s.Model = v
			}
		}
	}

	if raw, ok := m["temperature"]; ok {
		if string(raw) == "null" {
			s.Temperature = nil
		} else {
			var v float64
			if err := json.Unmarshal(raw, &v); err != nil {
				badJSON(w, err)
				return
			}
			s.Temperature = &v
		}
	}

	// commit_types: present non-null sets, explicit null clears.
	if raw, ok := m["commit_types"]; ok {
		if string(raw) == "null" {
			s.CommitTypes = ""
		} else {
			var v string
			if err := json.Unmarshal(raw, &v); err != nil {
				badJSON(w, err)
				return
			}
			s.CommitTypes = strings.Join(engine.ParseCommitTypes(v), ",")
		}
	}

	// mode: present non-null sets (must be lite|deep), explicit null clears.
	if raw, ok := m["mode"]; ok {
		if string(raw) == "null" {
			s.Mode = ""
		} else {
			var v string
			if err := json.Unmarshal(raw, &v); err != nil {
				badJSON(w, err)
				return
			}
			if !validMode(v) {
				writeErr(w, http.StatusBadRequest, "invalid mode")
				return
			}
			s.Mode = v
		}
	}

	// max_tokens: present value sets, explicit null clears to 0 (inherit);
	// integer >= 1 required.
	if err := setPresenceInt(m, "max_tokens", &s.MaxTokens, 1); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}

	// thinking_level: present non-null sets, explicit null clears to ""
	// (inherit); "" | off | low | medium | high accepted.
	if err := setPresenceStr(m, "thinking_level", &s.ThinkingLevel, thinkingLevelValues...); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}

	// llm_base_url: present non-null sets, explicit null clears (revert to env).
	// A cleared value must not round-trip as empty, or the endpoint would
	// silently break generation; an explicit null is the only "revert to env".
	if raw, ok := m["llm_base_url"]; ok {
		if string(raw) == "null" {
			s.BaseURL = ""
		} else {
			var v string
			if err := json.Unmarshal(raw, &v); err != nil {
				badJSON(w, err)
				return
			}
			if v = strings.TrimSpace(v); v == "" {
				writeErr(w, http.StatusBadRequest, "llm_base_url must not be empty")
				return
			}
			if err := llm.ValidateBaseURL(v); err != nil {
				writeErr(w, http.StatusBadRequest, "llm_base_url not allowed: "+err.Error())
				return
			}
			s.BaseURL = v
		}
	}

	// llm_api_key: present non-null sets, explicit null clears. No trim and no
	// non-empty requirement — an endpoint without auth is valid.
	if raw, ok := m["llm_api_key"]; ok {
		if string(raw) == "null" {
			s.APIKey = ""
		} else {
			var v string
			if err := json.Unmarshal(raw, &v); err != nil {
				badJSON(w, err)
				return
			}
			s.APIKey = v
		}
	}

	if err := a.store.UpsertSettings(s); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, a.settingsResp(s))
}

// handleGetModels proxies the LLM endpoint's /v1/models so the UI can offer a
// model dropdown. The effective endpoint (saved DB overrides env) is used, and
// the key is sent only when configured.
func (a *api) handleGetModels(w http.ResponseWriter, r *http.Request) {
	s, err := a.store.GetSettings()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	baseURL, apiKey := a.effectiveLLM(s)
	if baseURL == "" || a.llm == nil {
		writeErr(w, http.StatusServiceUnavailable, "llm base url not configured")
		return
	}
	// Defense in depth: a saved value may predate the settings PUT guard.
	if err := llm.ValidateBaseURL(baseURL); err != nil {
		writeErr(w, http.StatusServiceUnavailable, "llm base url not allowed")
		return
	}
	ids, err := a.llm.ListModels(r.Context(), baseURL, apiKey)
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	if ids == nil {
		ids = []string{}
	}
	writeJSON(w, http.StatusOK, ids)
}
