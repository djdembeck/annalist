# Agents

## Publication changes

Read [README.md](README.md), "Background", "Dashboard", and "Generation history", for the canonical generation/publication behavior.

- Preserve webhook deduplication versus deliberate forced CLI regeneration.
- Record generation history before starting remote work; failure to create that history prevents remote work.
- Report publication truthfully when later local persistence fails: a successful remote write must not become a false unpublished result. Preserve its warning outcome.
- Check the remote release before retrying publication after an ambiguous persistence failure; never blindly publish again.

## HTTP changes

Read `internal/httpapi/router.go` before changing routes. Keep the admin API authentication boundary distinct from platform webhooks, which verify their own signatures.
