package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/djdembeck/annalist/internal/db"
	"github.com/djdembeck/annalist/internal/pipeline"
	"github.com/djdembeck/annalist/internal/version"
)

func validPlatform(p string) bool {
	return p == "github" || p == "forgejo"
}

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

// effective is the resolved (inherited) tone/model/temperature preview.
type effective struct {
	Tone        string  `json:"tone"`
	Model       string  `json:"model"`
	Temperature float64 `json:"temperature"`
}

// repoItemResp is the JSON shape for a repo in /api/repos and the settings PUT.
type repoItemResp struct {
	Platform     string    `json:"platform"`
	Owner        string    `json:"owner"`
	Repo         string    `json:"repo"`
	Enabled      bool      `json:"enabled"`
	Tone         string    `json:"tone"`
	Instructions string    `json:"instructions"`
	Model        string    `json:"model"`
	Temperature  *float64  `json:"temperature"`
	Trigger      string    `json:"trigger"`
	Effective    effective `json:"effective"`
}

// repoItem builds the response for a repo_settings row, computing the
// effective (inherited) values via the pipeline's Resolve.
func (a *api) repoItem(ctx context.Context, row db.RepoSetting) (repoItemResp, error) {
	_, eff, err := a.pip.Resolve(ctx, row.Platform, row.Owner, row.Repo)
	if err != nil {
		return repoItemResp{}, err
	}
	return repoItemResp{
		Platform:     row.Platform,
		Owner:        row.Owner,
		Repo:         row.Repo,
		Enabled:      row.Enabled,
		Tone:         row.Tone,
		Instructions: row.Instructions,
		Model:        row.Model,
		Temperature:  row.Temperature,
		Trigger:      row.Trigger,
		Effective: effective{
			Tone:        eff.Tone,
			Model:       eff.Model,
			Temperature: eff.Temperature,
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
	items := make([]repoItemResp, 0, len(rows))
	for _, row := range rows {
		item, err := a.repoItem(ctx, row)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		items = append(items, item)
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
	if a.cfg.GitHubEnabled() && a.gh != nil {
		available, err = a.listAvailable(ctx, "github", a.gh, managedSet, available)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	if a.cfg.ForgejoEnabled() && a.fj != nil {
		available, err = a.listAvailable(ctx, "forgejo", a.fj, managedSet, available)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
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

	setting := db.RepoSetting{
		Platform: req.Platform,
		Owner:    req.Owner,
		Repo:     req.Repo,
		Enabled:  true,
		Trigger:  "auto",
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
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		badJSON(w, err)
		return
	}
	if strings.TrimSpace(req.ToTag) == "" {
		writeErr(w, http.StatusBadRequest, "to_tag is required")
		return
	}

	publish := true
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
	}, pipeline.Options{Force: req.Force, Publish: publish})
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

// settingsResp is the global-settings response, including the LLM/platform block.
func (a *api) settingsResp(s db.Settings) map[string]any {
	return map[string]any{
		"tone":         s.Tone,
		"instructions": s.Instructions,
		"model":        s.Model,
		"temperature":  s.Temperature,
		"llm": map[string]string{
			"base_url": a.cfg.LLM.BaseURL,
			"model":    a.cfg.LLM.Model,
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

	if err := a.store.UpsertSettings(s); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, a.settingsResp(s))
}
