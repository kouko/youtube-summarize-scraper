# CLAUDE.md — project notes for agents

Repo-level knowledge for future Claude sessions. User-facing usage lives in
[README.md](README.md) and [config.example.yaml](config.example.yaml); this file
records agent-relevant conventions and traps.

## LLM providers

- Provider construction: `summarizer.newSingleProvider` (`summarizer/summarizer.go`).
  Fixed backends (`ollama`, `llamacpp`, `claude-api`, `claude-code`, `gemini-cli`,
  `antigravity-cli`, `qwen-code`) are one config block each.
- **`openai-compat` is a `map[string]OpenAICompatConfig`** (`config/config.go`), not a
  single struct:
  - bare `openai-compat` → instance key `default`;
  - `openai-compat:<name>` → named instance (split on the first `:`);
  - missing instance (incl. bare with no `default`) → fail-loud error naming the instance.
- **High availability is free** — list instances in `llm.provider` and the existing
  `FallbackSummarizer` + per-name circuit breaker handle failover. Do **not** add bespoke
  scheduling / load-balancing / auto-discovery (explicitly out of scope; see
  `docs/loom/specs/2026-06-29-lmstudio-ha-multi-instance.md`).
- Instance names: user-defined, unlimited; `default` is reserved; must not contain `:`
  (documented constraint, not code-enforced).

## Gotchas

- **`yaml.v3` merges map keys** (does not replace) when `config.Load` unmarshals a user
  file into a struct seeded by `DefaultConfig()`. So **do not seed maps in
  `DefaultConfig()`** — a seeded entry becomes a phantom that survives into every loaded
  config. This is why `openai-compat` has **no** seeded `default` (seeding it once made the
  "bare with no default → error" contract unreachable). See PR #61.

## Conventions

- Commit trailer: use `Generated-by:`, not `Co-Authored-By:`.
- TDD: failing test first (loom-code `tdd-iron-law`).
