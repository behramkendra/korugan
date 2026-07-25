#!/usr/bin/env bash
# Seed GitHub milestones and starter issues for Korugan.
#
# Run this once, after the repo exists on GitHub and you've authenticated:
#   gh auth login
#   REPO=behramkendra/korugan bash scripts/seed-github.sh
#
# Idempotency: re-running creates duplicate issues. Run once.
set -euo pipefail

REPO="${REPO:-behramkendra/korugan}"
echo "Seeding $REPO ..."

# --- milestones (P1..P5 mirror ROADMAP.md) ---
create_ms() {
  gh api "repos/$REPO/milestones" -f title="$1" -f description="$2" >/dev/null 2>&1 \
    && echo "milestone: $1" || echo "milestone exists/skip: $1"
}
create_ms "P1 — Cloudflare read-only MVP" "Connect a Cloudflare account and get grounded explanations of real traffic."
create_ms "P2 — Recommend & approved apply" "Finding → recommendation → approve → apply → verify → rollback."
create_ms "P3 — Multi-provider" "Prove the connector abstraction with a second and third provider."
create_ms "P4 — Scale & autonomy (L3)" "Policy engine, ClickHouse/NATS when metrics demand, L3 opt-in."
create_ms "P5 — Self-hosted edge (agents)" "Nginx/Traefik/HAProxy/K8s Ingress via a lightweight agent."

# helper: create an issue with labels + milestone
issue() {
  local title="$1" body="$2" labels="$3" milestone="$4"
  gh issue create --repo "$REPO" --title "$title" --body "$body" \
    --label "$labels" --milestone "$milestone" >/dev/null \
    && echo "issue: $title"
}

issue "Web UI: onboarding, zone overview, event explorer, findings, chat" \
"Build the Next.js UI (Tailwind + shadcn/ui) against the existing REST API (docs/API.md). First screens: connect Cloudflare token, zone overview, event explorer with filters, findings feed, grounded chat. SSE for streaming chat comes with the API's streaming endpoint." \
"frontend,enhancement" "P1 — Cloudflare read-only MVP"

issue "Cloudflare: cache-rules and rate-limit-rules write paths" \
"Extend the Cloudflare writer (internal/connector/cloudflare/write.go) beyond WAF custom rules and cache.purge to cache rules and rate-limiting rules via the Rulesets API, with rollback material and conformance-style fixture tests." \
"connector,enhancement" "P2 — Recommend & approved apply"

issue "SSE streaming for chat responses" \
"Stream AI chat token-by-token over Server-Sent Events instead of a single JSON response. Update the /api/v1/chat endpoint and the engine to support a streaming callback." \
"ai-engine,enhancement" "P1 — Cloudflare read-only MVP"

issue "Sealed credential storage + settings API (replace env-only tokens)" \
"Today connector and LLM credentials come from env vars. Add a settings API that stores them sealed (internal/crypto) in the secrets table, masked in responses, with a two-token pattern (read token at onboarding, write token only when enabling L2)." \
"security,enhancement" "P1 — Cloudflare read-only MVP"

issue "AWS CloudFront connector (design review first)" \
"Map CloudFront onto docs/CONNECTORS.md: distributions as resources, WAF via AWS WAF, logs/metrics for events. Open with a design-review issue identifying where the normalized schema strains before writing code." \
"connector,design-review" "P3 — Multi-provider"

issue "Connector conformance test suite" \
"A public, runnable suite (internal/connector/conformance) every connector must pass: interface contract, event normalization golden files, cursor resume, idempotent apply under retry, rollback round-trip per action type, credential redaction." \
"connector,enhancement" "P3 — Multi-provider"

issue "Policy engine for L3 autonomy" \
"Implement the Policy type end to end: action-type allowlist, per-resource rate limits, quiet hours, mandatory reversibility, kill switch. Gate L3 execution on a matching enabled policy plus an L2 track record." \
"enhancement" "P4 — Scale & autonomy (L3)"

issue "OpenTelemetry traces + metrics wiring" \
"Flesh out internal/obs with real OTLP export: spans for every connector call, LLM call (model/tokens/latency/cost) and action execution; basic dashboards and SLOs for connector freshness and action latency." \
"observability,enhancement" "P4 — Scale & autonomy (L3)"

issue "Model price table + per-workspace budget dashboard" \
"Ship an editable model price table (USD/1k tokens) and a usage/budget view backed by llm_usage, so BYOK users see spend per provider/model/task-class and can set caps in the UI." \
"ai-engine,enhancement" "P2 — Recommend & approved apply"

echo "Done."
