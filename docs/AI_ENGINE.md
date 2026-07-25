# AI Engine

The AI engine turns normalized edge data into explanations, findings and provider-ready recommendations. It is an **orchestration layer over models the user brings** — Korugan trains nothing, hosts nothing, and pays for no inference.

## BYOK — Bring Your Own Key

Design constraint, stated bluntly: **the project cannot and will not subsidize inference.** Every deployment connects the user's own LLM account(s); costs go straight to their provider. This is also what makes self-hosting private: prompts flow from *your* server to *your* chosen API (or never leave the machine, with Ollama).

### Supported providers (v0.1)

| Provider | Adapter | Notes |
|---|---|---|
| **OpenRouter** | openai-compatible | Recommended default: one key, hundreds of models, per-model pricing visible; ideal for tiering |
| **Anthropic** | native (Messages API) | First-class tool-use support |
| **OpenAI** | openai-compatible | Also the template for any compatible endpoint |
| **DeepSeek** | openai-compatible | Very low cost; excellent routine-summarization tier |
| **Ollama / local** | openai-compatible | Free, offline, zero data egress |

Two real adapters cover all five: `anthropic` and `openai-compatible` (base URL + auth header + quirks table: which endpoints support tool calling, JSON mode, streaming). Adding a future provider is a config entry or a small quirk patch, not a new subsystem.

```go
type LLMProvider interface {
    // Complete runs one model turn; streams via the callback when cb != nil.
    Complete(ctx context.Context, req CompletionRequest, cb StreamFunc) (*Completion, error)
    // Models lists what this key can access (used by the tiering UI).
    Models(ctx context.Context) ([]ModelInfo, error)
    // Ping validates the key cheaply at save time.
    Ping(ctx context.Context) error
}
// CompletionRequest: messages, tool definitions, model id, max_tokens,
// temperature, metadata (task_class, workspace) — provider-neutral.
```

### Key management

- Keys entered by the user in the UI (or `KORUGAN_LLM_*` env vars for single-user self-host); **never** requested by, shown to, or transitable through the AI itself.
- Sealed with AES-256-GCM; master key from `KORUGAN_MASTER_KEY` (32 bytes, generated at first boot if absent, with a loud warning to back it up). Per-secret random nonce; ciphertext in Postgres.
- Displayed masked (`sk-or-…a1b2`), redacted from logs/traces/error messages by the shared redaction middleware; a leaked-key regression test lives in CI.
- Multiple keys per workspace (e.g. OpenRouter + Ollama); each key validated with `Ping` on save and re-validated on schedule so dead keys surface as findings, not silent failures.

### Model tiering

Different jobs deserve different price points. Task classes map to tiers; tiers map to user-chosen models with sensible defaults per provider:

| Task class | Tier | Default behavior |
|---|---|---|
| `summarize_events` (rolling digests) | fast | cheapest configured model (e.g. a DeepSeek/Haiku-class model) |
| `explain_finding` | balanced | mid-tier model |
| `chat` (interactive Q&A) | balanced | mid-tier, streaming |
| `generate_rule` (WAF/cache/ratelimit drafts) | deep | strongest configured model |
| `plan_remediation` (multi-step, L2/L3 material) | deep | strongest configured model |

Users can pin any class to any model. If only one model is configured, everything uses it. Model names are configuration, not code — defaults ship as an editable table, because model catalogs age fast.

### Cost controls

- **Accounting:** every call stores model, input/output tokens, latency and estimated cost (provider price table, user-overridable) in `llm_usage` — queryable per workspace/day/task class in the UI.
- **Budgets:** per-workspace daily and monthly caps. Soft threshold warns (finding), hard cap stops AI calls (`budget.exhausted` finding; rule-based analyzers keep running). Interactive chat gets a small reserved slice so a burned batch budget doesn't kill the ability to ask what happened.
- **Caching & dedup:** analysis requests keyed by hash(task class, model, input window); identical windows reuse stored results. Rolling digests batch events so routine summarization is O(windows), not O(events).
- **Zero-key mode:** with no key configured, Korugan is a fully functional collector/dashboard with rule-based findings. AI features light up when a key arrives. No dark patterns, no trial inference.

## Agent loop

One loop, deliberately simple:

```
context assembly → model turn → tool calls (parallel-safe) → model turn → … → structured output
```

- **Context assembly** builds the system prompt (role, autonomy rules, output schemas) plus task context: resource metadata, relevant finding, and *references* to data windows — the model pulls actual data through tools, keeping prompts small and audits precise.
- **Bounded:** max tool-call rounds and max token spend per run (task-class dependent); overruns produce a truncated-but-honest result, never a silent retry storm.
- **Every run persists** its full trace: prompts, tool calls + arguments, tool results (hashes + row counts for big ones), output, token/cost figures. The audit chain for an applied action reaches back to the exact events the model saw.

### Tools

Read tools (available from L0):

| Tool | Returns |
|---|---|
| `query_events` | filtered/aggregated events (time window, category, resource; capped row counts) |
| `get_snapshot` | current normalized config for a resource |
| `get_analytics` | pre-aggregated traffic series (from rollups) |
| `get_findings` | open findings + history for a resource |
| `search_docs` | Korugan's own provider-knowledge notes (curated, versioned in-repo) |

Write-adjacent tool (L1+): `propose_action` — validates params against the action-type JSON Schema, computes the diff via the connector's `DryRun`, and files a **Recommendation**. It never applies anything. There is deliberately no `apply` tool: application happens in the action pipeline (approval → executor), outside any LLM context.

### Guardrails

1. **Credential isolation.** The model calls internal tools only; tools run under the workspace's capability + autonomy context; connector credentials are never in scope. Even a fully compromised prompt cannot exfiltrate keys or call a provider API.
2. **Prompt-injection defense.** Everything from the wire — paths, user agents, hostnames, rule names, log lines — is untrusted. It enters prompts only inside delimited data blocks tagged as untrusted content; system rules instruct the model to treat such content as data, never instructions. Tool results are schema-validated on the way out; `propose_action` params face strict JSON Schema + domain validation (an injected "delete all DNS records" fails on action-type allowlist, reversibility check, and scope rules — defense in depth, not vibes).
3. **Blast-radius limits in the domain layer.** Zone-wide-match WAF expressions flagged; irreversible action types unproposable; per-resource action rate limits. Enforced in Go, not in the prompt.
4. **Grounding requirements.** Explanations must cite event IDs / windows they derive from; the UI renders citations as links into the event explorer. Uncited claims render with a visible "unverified" badge — the schema forces the model to separate evidence from inference.
5. **Model output is a proposal, always.** Autonomy levels gate the *pipeline* (see ARCHITECTURE.md); nothing the model emits changes its own permissions, budgets, or level.

## Approval flow (L2)

```
Finding ──► Recommendation (diff + rationale + blast radius + rollback plan + confidence)
        ──► Human review (UI): approve / edit params / reject-with-reason
        ──► Executor: precondition re-check → provider dry-run → apply
        ──► Verification window: watch error budget on affected resource
        ──► Healthy: close loop. Regressed: automatic rollback + incident finding.
```

Rejection reasons feed back as few-shot context for future `generate_rule` runs in that workspace — the cheapest personalization that actually works, no fine-tuning involved.

## Failure posture

LLM unavailable / key dead / budget exhausted → AI features degrade to zero-key mode; ingest, rule-based analyzers, dashboards, and previously approved-but-unexecuted actions (which never depended on a model) continue. The AI engine is an enhancement layer; its outage must never be a platform outage.

## Evaluation (grows with the project)

`ai/evals/` holds golden tasks per task class (sanitized real windows → expected findings/rules). Run manually per PR touching prompts/tools in early phases; CI-gated once stable. Model-comparison results land in docs so BYOK users can pick models on evidence, not fashion.
