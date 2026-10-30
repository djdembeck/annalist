package httpapi

import (
	"context"
	"net/http"
	"strconv"

	"github.com/djdembeck/annalist/internal/db"
)

const (
	defaultRunLimit = 20
	maxRunLimit     = 100
)

func (a *api) repoItemWithLatest(ctx context.Context, row db.RepoSetting) (repoItemResp, error) {
	item, err := a.repoItem(ctx, row)
	if err != nil {
		return repoItemResp{}, err
	}
	latest, err := a.store.LatestRun(ctx, row.Platform, row.Owner, row.Repo)
	if err != nil {
		return repoItemResp{}, err
	}
	item.LatestRun = latest
	return item, nil
}

func nullableCode(code string) any {
	if code == "" {
		return nil
	}
	return code
}

func (a *api) handleListRuns(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	platform, owner, repo := q.Get("platform"), q.Get("owner"), q.Get("repo")
	if q.Has("platform") || q.Has("owner") || q.Has("repo") {
		if !validPlatform(platform) || owner == "" || repo == "" {
			writeErr(w, http.StatusBadRequest, "platform, owner and repo must specify a complete repository")
			return
		}
	}
	limit := defaultRunLimit
	if q.Has("limit") {
		var err error
		limit, err = strconv.Atoi(q.Get("limit"))
		if err != nil || limit < 1 || limit > maxRunLimit {
			writeErr(w, http.StatusBadRequest, "limit must be an integer between 1 and 100")
			return
		}
	}
	runs, err := a.store.ListRuns(platform, owner, repo, limit)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not load generation history")
		return
	}
	writeJSON(w, http.StatusOK, runs)
}
