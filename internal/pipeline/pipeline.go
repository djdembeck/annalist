package pipeline

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/djdembeck/annalist/internal/config"
	"github.com/djdembeck/annalist/internal/db"
	"github.com/djdembeck/annalist/internal/engine"
	"github.com/djdembeck/annalist/internal/llm"
)

// ErrNotFound is returned by platform calls (repo file reads, release lookup)
// when the requested resource does not exist.
var ErrNotFound = errors.New("not found")

// OwnerRepo is a repository reference returned by platform listings so other
// packages can join with repo_settings without importing platform packages.
type OwnerRepo struct {
	Owner        string
	Repo         string
	Fork         bool
	OwnNamespace bool
	UpdatedAt    time.Time
	PushedAt     time.Time
}

// Effective is the runtime-resolved LLM endpoint: saved DB settings win
// over env/config. An empty BaseURL/APIKey means "use the env/config value".
type Effective struct {
	BaseURL string
	APIKey  string
}

// Spec identifies a single release for which to generate notes.
type Spec struct {
	Platform  string
	Owner     string
	Repo      string
	ToTag     string
	FromTag   string // optional; empty means auto-resolve the previous tag
	ReleaseID string
	// Profile is the optional named release-note profile (see
	// .annalist/release-notes.yaml). Empty keeps the legacy
	// provider-specific instructions path; a non-empty value selects the
	// profile's prompt as the entire system prompt.
	Profile string
	// DisplayVersion is the optional presentation version for the generated
	// notes; empty falls back to ToTag.
	DisplayVersion string
}

// Result is the structured output of GenerateNotes: the notes text plus the
// generation-contract metadata needed for cache identity and API responses.
type Result struct {
	Notes          string
	Profile        string
	DisplayVersion string
	FromTag        string
	ToTag          string
	ConfigDigest   string
}

// Options controls generation behavior.
type Options struct {
	Force   bool // bypass idempotency and the don't-clobber guard
	Publish bool // write the notes into the release body
	// Mode overrides the resolved mode for this invocation only; empty = use resolved.
	Mode string
}

// Release is the small platform-agnostic view of a release.
type Release struct {
	ID   int64
	Body string
}

// RepoFileReader reads a file from a repository using platform credentials.
// ref pins the read to a git ref (e.g. the source tag); ref == "" means the
// repository default branch.
type RepoFileReader interface {
	ReadRepoFile(ctx context.Context, owner, repo, path, ref string) (string, error)
}

// Platform is everything the pipeline needs from a hosting platform.
type Platform interface {
	RepoFileReader
	GetReleaseByTag(ctx context.Context, owner, repo, tag string) (*Release, error)
	EditReleaseBody(ctx context.Context, owner, repo string, releaseID int64, body string) error
	CloneInfo(ctx context.Context, owner, repo string) (cloneURL, header string, err error)
}

// Pipeline drives the whole generate flow: resolve settings, clone,
// collect the commit log, call the LLM, and optionally publish.
type Pipeline struct {
	Cfg     *config.Config
	DB      *db.Store
	LLM     *llm.Client
	Engine  *engine.Engine
	GitHub  Platform
	Forgejo Platform

	// generationInflight serializes generation per cache_key (the full
	// generation contract: range, profile, display version, config digest) so
	// concurrent identical requests cannot double-spend the LLM. Different
	// profiles of the same release use different keys and run concurrently.
	// Keyed locks are removed once the last participant exits, so long-running
	// services do not accumulate one lock per unique contract.
	generationInflight     *keyedLock
	generationInflightOnce sync.Once

	// publishInflight serializes release-body replacement per release
	// identity (platform/owner/repo@tag) across all request surfaces, so two
	// profile publishes cannot interleave fetch/edit (last-write-wins).
	publishInflight     *keyedLock
	publishInflightOnce sync.Once

	// genSem bounds concurrent generations (clone + LLM + publish) so a burst
	// of signed deliveries cannot exhaust CPU, disk, or LLM budget.
	genSemOnce sync.Once
	genSem     chan struct{}
}

// sem returns the generation semaphore, initializing it lazily so tests can
// construct a Pipeline literal without going through New.
func (p *Pipeline) sem() chan struct{} {
	p.genSemOnce.Do(func() { p.genSem = make(chan struct{}, 4) })
	return p.genSem
}

// genInflight returns the keyed lock that serializes generation per cache key,
// initializing it lazily so tests can construct a Pipeline literal without
// going through New.
func (p *Pipeline) genInflight() *keyedLock {
	p.generationInflightOnce.Do(func() { p.generationInflight = newKeyedLock() })
	return p.generationInflight
}

// pubInflight returns the keyed lock that serializes release-body replacement
// per release identity, initializing it lazily.
func (p *Pipeline) pubInflight() *keyedLock {
	p.publishInflightOnce.Do(func() { p.publishInflight = newKeyedLock() })
	return p.publishInflight
}

// keyedLock is a map of per-key mutexes whose entries are reaped once the
// last participant for a key leaves. Reaping is safe because every change to
// a state's participant count and every map insert/delete happens under the
// table mutex (kl.mu); the state mutex (st.mu) protects only the critical
// section and is never held while touching the count or the map:
//
//   - Lock bumps the count under kl.mu, then takes st.mu for the section.
//   - Unlock drops the count under kl.mu, deletes the entry if the count
//     reached zero, and releases st.mu while still holding kl.mu.
//
// The increment and the last decrement are therefore mutually exclusive, and
// the count is never zero while a participant is alive or waiting: a late
// arrival that races the reaper either takes kl.mu before the departing
// holder (bumps the still-live state and is serialized behind it on st.mu)
// or takes it only after the holder has fully left the critical section
// (finds the key reaped and creates a fresh state). Two live participants
// for the same key can never hold different mutexes.
type keyedLock struct {
	mu    sync.Mutex
	locks map[string]*lockState
}

// lockState is the per-key participant count (guarded by the table mutex) and
// the mutex shared by every participant that holds that key.
type lockState struct {
	count int
	mu    sync.Mutex
}

func newKeyedLock() *keyedLock { return &keyedLock{locks: make(map[string]*lockState)} }

// Lock acquires the mutex for key. Callers must pair it with Unlock(key) on
// the same keyedLock.
func (kl *keyedLock) Lock(key string) {
	kl.mu.Lock()
	st, ok := kl.locks[key]
	if !ok {
		st = &lockState{}
		kl.locks[key] = st
	}
	st.count++
	kl.mu.Unlock()

	st.mu.Lock()
}

// Unlock releases the mutex for key and reaps the entry once the last
// participant leaves. It holds the table mutex through the release of the
// state mutex, so a concurrent Lock(key) either takes the table mutex first
// (bumps the still-live state and blocks on the state mutex behind this
// holder) or takes it only after this holder has fully left the critical
// section (finds the key reaped and creates a fresh state).
func (kl *keyedLock) Unlock(key string) {
	kl.mu.Lock()
	st := kl.locks[key]
	st.count--
	if st.count == 0 {
		delete(kl.locks, key)
	}
	st.mu.Unlock()
	kl.mu.Unlock()
}

// count returns the number of live keys; used by tests to assert reaping.
func (kl *keyedLock) count() int {
	kl.mu.Lock()
	defer kl.mu.Unlock()
	return len(kl.locks)
}

// state returns the live state for key, or nil if reaped; used by tests to
// verify reaping handshakes.
func (kl *keyedLock) state(key string) *lockState {
	kl.mu.Lock()
	defer kl.mu.Unlock()
	return kl.locks[key]
}

// marker is written at the end of every release body this app publishes, so
// regen can identify bodies the app owns.
const marker = "<!-- generated by annalist -->"

// New builds a Pipeline. gh and fj may be nil when the platform is not
// configured; GenerateNotes reports a clear error when used anyway.
func New(cfg *config.Config, store *db.Store, llmClient *llm.Client, gh, fj Platform) *Pipeline {
	return &Pipeline{
		Cfg:     cfg,
		DB:      store,
		LLM:     llmClient,
		Engine:  &engine.Engine{LLM: llmClient},
		GitHub:  gh,
		Forgejo: fj,
	}
}

func (p *Pipeline) platformFor(platform string) (Platform, error) {
	switch platform {
	case "github":
		if p.GitHub == nil {
			return nil, errors.New("pipeline: platform github is not configured")
		}
		return p.GitHub, nil
	case "forgejo":
		if p.Forgejo == nil {
			return nil, errors.New("pipeline: platform forgejo is not configured")
		}
		return p.Forgejo, nil
	default:
		return nil, fmt.Errorf("pipeline: unknown platform %q", platform)
	}
}

// Resolve computes the effective settings for a repo. Per-repo row values
// override the global settings, which fall back to the configured LLM
// defaults, and (for commit types only) to the recommended default types.
// The in-repo instructions file is resolved separately inside
// GenerateNotes (highest precedence). The Effective return carries the
// runtime-resolved LLM endpoint (saved DB base URL / API key win over
// env/config) for the caller to pass through to generation.
func (p *Pipeline) Resolve(ctx context.Context, platform, owner, repo string) (bool, Effective, engine.Resolved, error) {
	global, err := p.DB.GetSettings()
	if err != nil {
		return false, Effective{}, engine.Resolved{}, fmt.Errorf("pipeline: global settings: %w", err)
	}

	row, err := p.DB.GetRepoSettings(platform, owner, repo)
	if err != nil {
		return false, Effective{}, engine.Resolved{}, fmt.Errorf("pipeline: repo settings: %w", err)
	}

	enabled := true
	tone := global.Tone
	instructions := global.Instructions
	model := global.Model
	var temperature *float64 = global.Temperature

	if row != nil {
		enabled = row.Enabled
		if row.Tone != "" {
			tone = row.Tone
		}
		if row.Instructions != "" {
			instructions = row.Instructions
		}
		if row.Model != "" {
			model = row.Model
		}
		if row.Temperature != nil {
			temperature = row.Temperature
		}
	}

	if model == "" {
		model = p.Cfg.LLM.Model
	}

	// Effective endpoint: env/config is the floor; a non-empty saved DB value
	// (base URL / API key) overrides it.
	eff := Effective{
		BaseURL: p.Cfg.LLM.BaseURL,
		APIKey:  p.Cfg.LLM.APIKey,
	}
	if global.BaseURL != "" {
		eff.BaseURL = global.BaseURL
	}
	if global.APIKey != "" {
		eff.APIKey = global.APIKey
	}

	// Resolve commit types with same precedence: repo → global → config →
	// default. When nothing is saved or configured, the recommended default
	// types (engine.DefaultCommitTypes) apply; only an explicit "*" keeps
	// all commit types.
	commitTypes := global.CommitTypes
	if row != nil && row.CommitTypes != "" {
		commitTypes = row.CommitTypes
	}
	if commitTypes == "" {
		commitTypes = p.Cfg.LLM.CommitTypes
	}
	if commitTypes == "" {
		commitTypes = engine.DefaultCommitTypes
	}

	// Mode precedence: repo row → global row → default lite.
	mode := global.Mode
	if row != nil && row.Mode != "" {
		mode = row.Mode
	}
	if mode == "" {
		mode = engine.ModeLite
	}

	// Max tokens precedence: repo row → global row → config (0 = unset).
	maxTokens := global.MaxTokens
	if row != nil && row.MaxTokens > 0 {
		maxTokens = row.MaxTokens
	}
	if maxTokens == 0 {
		maxTokens = p.Cfg.LLM.MaxTokens
	}

	// Thinking level precedence: repo row → global row → config. Empty
	// inherits down the chain; "off" is a stored value that terminates the
	// chain — an explicit UI off must not be overridden by an env/config
	// level. "off" stays as-is here; the llm client translates it to the
	// wire value.
	thinkingLevel := global.ThinkingLevel
	if row != nil && row.ThinkingLevel != "" {
		thinkingLevel = row.ThinkingLevel
	}
	if thinkingLevel == "" {
		thinkingLevel = p.Cfg.LLM.ThinkingLevel
	}

	resolved := engine.Resolved{
		Tone:          tone,
		Instructions:  instructions,
		Model:         model,
		Temperature:   p.Cfg.LLM.Temperature,
		CommitTypes:   engine.ParseCommitTypes(commitTypes),
		Mode:          mode,
		MaxTokens:     maxTokens,
		ThinkingLevel: thinkingLevel,
	}
	if temperature != nil {
		resolved.Temperature = *temperature
	}

	return enabled, eff, resolved, nil
}

// GenerateNotes produces release notes for spec. The Result is returned even
// when opts.Publish is false. Errors (LLM failure, clone failure, API
// failure) propagate — no fallback text — and idempotency + manual regen
// cover retries.
//
// The flow: validate the request, resolve settings, clone, resolve the range,
// load the tag-pinned instructions (named profile or legacy file), compute
// the config digest and cache key, then under the per-contract generation
// lock recheck the cache (unless forced), generate, and optionally publish.
// The cache lookup happens after the lock and only when everything needed
// for the key has been resolved, because range and prompt identity are part
// of the key. Generation is bounded by a semaphore; per-contract locks make
// identical concurrent requests share one LLM call, and per-release publish
// locks serialize release-body replacement.
func (p *Pipeline) GenerateNotes(ctx context.Context, spec Spec, opts Options) (Result, error) {
	// Shared request validation (used by the API and the CLI alike).
	if err := validateProfileRequest(spec.Profile); err != nil {
		return Result{}, fmt.Errorf("pipeline: %w", err)
	}
	if spec.DisplayVersion != "" {
		dv, err := validateDisplayVersion(spec.DisplayVersion)
		if err != nil {
			return Result{}, fmt.Errorf("pipeline: %w", err)
		}
		spec.DisplayVersion = dv
	}

	select {
	case p.sem() <- struct{}{}:
		defer func() { <-p.sem() }()
	case <-ctx.Done():
		return Result{}, ctx.Err()
	}

	// Resolve the effective settings before cloning or dialing: a saved base
	// URL that predates the settings guard (or an env value) would otherwise
	// be dialed with the bearer key. An empty effective URL means
	// unconfigured: llm.Chat then falls back to the client's configured base
	// URL, and a fully unconfigured endpoint fails at request time with a
	// host-less-URL error from http.Client.Do (not at validation).
	_, eff, resolved, err := p.Resolve(ctx, spec.Platform, spec.Owner, spec.Repo)
	if err != nil {
		return Result{}, err
	}
	if eff.BaseURL != "" {
		if err := llm.ValidateBaseURL(eff.BaseURL); err != nil {
			return Result{}, fmt.Errorf("pipeline: llm base url not allowed: %v", err)
		}
	}

	platform, err := p.platformFor(spec.Platform)
	if err != nil {
		return Result{}, err
	}

	cloneURL, header, err := platform.CloneInfo(ctx, spec.Owner, spec.Repo)
	if err != nil {
		return Result{}, fmt.Errorf("pipeline: clone info: %w", err)
	}

	workdir, cleanup, err := engine.CloneTo(ctx, p.Cfg.Data.Dir, spec.Platform, cloneURL, header)
	if err != nil {
		return Result{}, fmt.Errorf("pipeline: clone: %w", err)
	}
	defer cleanup()

	from := spec.FromTag
	if from == "" {
		from = engine.ResolvePrevTag(ctx, workdir, spec.ToTag)
	}
	// Git ref names cannot start with '-' (check-ref-format); rejecting such
	// tags here prevents option-injection into the git log invocation below.
	if strings.HasPrefix(from, "-") || strings.HasPrefix(spec.ToTag, "-") {
		return Result{}, fmt.Errorf("pipeline: invalid tag %q", spec.ToTag)
	}
	if opts.Mode != "" {
		resolved.Mode = opts.Mode
	}

	// Instructions are pinned to the source tag so the prompt a release was
	// generated with is the prompt that existed at that tag.
	if spec.Profile != "" {
		// A named profile's prompt is the entire system prompt; the
		// provider-specific legacy instructions file is intentionally not
		// read or composed.
		prompt, err := resolveProfile(ctx, platform, spec.Owner, spec.Repo, spec.Profile, spec.ToTag)
		if err != nil {
			return Result{}, fmt.Errorf("pipeline: %w", err)
		}
		resolved.Instructions = prompt
	} else {
		// Legacy path: the provider-specific in-repo instructions file has
		// the highest precedence; absent/unreadable falls back to the
		// resolved settings (fail-open, as before).
		instructionsPath := ".github/release-notes-instructions.md"
		if spec.Platform == "forgejo" {
			instructionsPath = ".forgejo/release-notes.md"
		}
		if content, ferr := platform.ReadRepoFile(ctx, spec.Owner, spec.Repo, instructionsPath, spec.ToTag); ferr == nil && content != "" {
			resolved.Instructions = content
		}
	}

	// The config digest covers the runtime generation contract — including
	// the effective system prompt (which subsumes tone/persona expansion),
	// model, temperature, commit types, mode, max tokens, thinking level,
	// and the effective LLM base URL. The API key is never part of it.
	configDigest := p.configDigest(eff.BaseURL, resolved)
	cacheKey := p.cacheKey(spec, from, configDigest)

	// Acquire the per-contract generation lock, then recheck the cache: the
	// key already encodes the resolved range and prompt identity, and the
	// lock makes the check-then-generate atomic for identical requests.
	gi := p.genInflight()
	gi.Lock(cacheKey)
	defer gi.Unlock(cacheKey)

	if !opts.Force {
		existing, err := p.DB.GetGenerated(cacheKey)
		if err != nil {
			return Result{}, fmt.Errorf("pipeline: check generated: %w", err)
		}
		if existing != nil {
			return Result{
				Notes:          existing.Notes,
				Profile:        spec.Profile,
				DisplayVersion: displayVersionFor(spec),
				FromTag:        from,
				ToTag:          spec.ToTag,
				ConfigDigest:   existing.ConfigDigest,
			}, nil
		}
	}

	commitLog := engine.CollectCommitLog(ctx, workdir, from, spec.ToTag, resolved.CommitTypes)

	var notes string
	if strings.TrimSpace(commitLog) == "" {
		notes = "No changes documented."
	} else {
		diff := ""
		if resolved.Mode == engine.ModeDeep {
			diff = engine.CollectDiff(ctx, workdir, from, spec.ToTag, engine.DiffBudgetBytes)
		}
		notes, err = p.Engine.Generate(ctx, resolved, eff.BaseURL, eff.APIKey, displayVersionFor(spec), spec.ToTag, from, commitLog, diff)
		if err != nil {
			return Result{}, err
		}
	}

	result := Result{
		Notes:          notes,
		Profile:        spec.Profile,
		DisplayVersion: displayVersionFor(spec),
		FromTag:        from,
		ToTag:          spec.ToTag,
		ConfigDigest:   configDigest,
	}

	if opts.Publish {
		if err := p.Publish(ctx, result, spec, platform, opts.Force); err != nil {
			return Result{}, err
		}
	}

	return result, nil
}

// displayVersionFor applies the fallback: an omitted presentation version is
// the source tag.
func displayVersionFor(spec Spec) string {
	if spec.DisplayVersion != "" {
		return spec.DisplayVersion
	}
	return spec.ToTag
}

// configDigestInput is the deterministic JSON over which the config digest
// hashes. Field order is fixed by encoding/json (struct order), so the
// digest is stable across calls and processes.
type configDigestInput struct {
	SystemPrompt  string   `json:"system_prompt"`
	Model         string   `json:"model"`
	Temperature   float64  `json:"temperature"`
	CommitTypes   []string `json:"commit_types"`
	Mode          string   `json:"mode"`
	MaxTokens     int      `json:"max_tokens"`
	ThinkingLevel string   `json:"thinking_level"`
	BaseURL       string   `json:"base_url"`
}

// configDigest is the lowercase full SHA-256 over the runtime generation
// contract. Compile-time constants (e.g. the diff byte budget) are
// deliberately excluded.
func (p *Pipeline) configDigest(baseURL string, resolved engine.Resolved) string {
	in := configDigestInput{
		SystemPrompt:  p.Engine.BuildSystemPrompt(resolved),
		Model:         resolved.Model,
		Temperature:   resolved.Temperature,
		CommitTypes:   resolved.CommitTypes,
		Mode:          resolved.Mode,
		MaxTokens:     resolved.MaxTokens,
		ThinkingLevel: resolved.ThinkingLevel,
		BaseURL:       baseURL,
	}
	if in.CommitTypes == nil {
		in.CommitTypes = []string{}
	}
	b, _ := json.Marshal(in)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// cacheKeyInput is the deterministic JSON over which the generated-note
// cache key hashes.
type cacheKeyInput struct {
	Platform       string `json:"platform"`
	Owner          string `json:"owner"`
	Repo           string `json:"repo"`
	ReleaseID      string `json:"release_id"`
	FromTag        string `json:"from_tag"`
	ToTag          string `json:"to_tag"`
	Profile        string `json:"profile"`
	DisplayVersion string `json:"display_version"`
	ConfigDigest   string `json:"config_digest"`
}

// cacheKey is the lowercase full SHA-256 over the full generation contract.
func (p *Pipeline) cacheKey(spec Spec, fromTag, configDigest string) string {
	in := cacheKeyInput{
		Platform:       spec.Platform,
		Owner:          spec.Owner,
		Repo:           spec.Repo,
		ReleaseID:      spec.ReleaseID,
		FromTag:        fromTag,
		ToTag:          spec.ToTag,
		Profile:        spec.Profile,
		DisplayVersion: displayVersionFor(spec),
		ConfigDigest:   configDigest,
	}
	b, _ := json.Marshal(in)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// Publish writes result.Notes into the release body for spec, honoring the
// don't-clobber guard unless force is set, then records the generated note.
// The release-body replacement is serialized per release identity —
// platform/owner/repo@tag, the same identity a release resolves to from any
// request surface (webhook, manual API, CLI) — so fetch/don't-clobber/edit/
// mark happen under one lock regardless of which surface issued the publish.
// Concurrent profile publishes therefore cannot interleave: they are
// last-write-wins by design, because each publish asks Annalist to replace the
// same owned body.
// publishLockKey is the surface-independent identity of a release:
// platform/owner/repo@tag. ReleaseID namespaces differ per request surface
// (webhook numeric id, "manual:…", "cli:…"), so keying the publish lock on it
// would only serialize within one surface; platform/owner/repo@tag is the same
// identity a release resolves to from any surface, so it is the correct
// serialization key.
func publishLockKey(spec Spec) string {
	return spec.Platform + "/" + spec.Owner + "/" + spec.Repo + "@" + spec.ToTag
}

func (p *Pipeline) Publish(ctx context.Context, result Result, spec Spec, platform Platform, force bool) error {
	pub := p.pubInflight()
	lockKey := publishLockKey(spec)
	pub.Lock(lockKey)
	defer pub.Unlock(lockKey)

	release, err := platform.GetReleaseByTag(ctx, spec.Owner, spec.Repo, spec.ToTag)
	if err != nil {
		return fmt.Errorf("pipeline: fetch release: %w", err)
	}

	if !force && release.Body != "" && !strings.Contains(release.Body, marker) {
		log.Printf("pipeline: %s/%s %s: human-edited release body; leaving it alone", spec.Owner, spec.Repo, spec.ToTag)
		return nil
	}

	body := result.Notes
	if !strings.HasSuffix(body, marker) {
		body = body + "\n" + marker
	}
	if err := platform.EditReleaseBody(ctx, spec.Owner, spec.Repo, release.ID, body); err != nil {
		return fmt.Errorf("pipeline: edit release body: %w", err)
	}

	note := db.GeneratedNote{
		CacheKey:       p.cacheKey(spec, result.FromTag, result.ConfigDigest),
		Platform:       spec.Platform,
		Owner:          spec.Owner,
		Repo:           spec.Repo,
		ReleaseID:      spec.ReleaseID,
		FromTag:        result.FromTag,
		ToTag:          spec.ToTag,
		Profile:        result.Profile,
		DisplayVersion: result.DisplayVersion,
		ConfigDigest:   result.ConfigDigest,
		Notes:          result.Notes,
	}
	if err := p.DB.MarkGenerated(note); err != nil {
		return fmt.Errorf("pipeline: record generated: %w", err)
	}
	return nil
}
