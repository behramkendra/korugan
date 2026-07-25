# Roadmap

Milestone-based, no dates. Each phase has explicit scope, non-goals, and a falsifiable "done when". Phases ship sequentially; a phase is not started while the previous one's done-criteria are open.

---

## P0 — Foundation (current)

**Goal:** the design is written down well enough that a stranger can disagree with it precisely.

- [x] Founding documentation set (this repo)
- [ ] Design review issues triaged; connector + event schema survives first contact with reviewers
- [ ] Repo hygiene: CI skeleton (lint + test), issue templates, labels

**Done when:** connector interface and event schema have survived at least one round of external review without structural change requests.

---

## P1 — Cloudflare, read-only (MVP)

**Goal:** connect a Cloudflare account and get real explanations about real traffic within 10 minutes of `docker compose up` / running the binary.

In scope:

- Cloudflare connector: token validation, zone sync, security events (WAF/firewall), HTTP analytics, DNS records, SSL/cert status
- Event normalization + Postgres storage (partitioned), dedup, retention config
- Rule-based analyzers (no LLM required): cert expiry, error-rate spikes, blocked-traffic anomalies, config drift
- BYOK LLM setup: OpenRouter, Anthropic, OpenAI-compatible (covers DeepSeek), Ollama; encrypted key storage; zero-key mode
- AI chat over your data: "what happened in the last 24h?", "why did blocks spike?" — read-only tools, grounded answers with event citations
- Minimal Next.js UI: onboarding, zone overview, event explorer, findings feed, chat
- Single-user auth (initial admin), session cookies

Out of scope: any write action against Cloudflare, multi-user, multi-provider.

**Done when:** a fresh user connects a real zone, and the answer to "why did my blocked traffic spike yesterday?" cites the actual rule IDs and event windows — verified against Cloudflare's own dashboard.

---

## P2 — Recommend & approved apply (L1 → L2)

**Goal:** close the loop from finding to fix without giving up human control.

In scope:

- Recommendation objects: provider-ready change + human-readable diff + rationale + blast-radius estimate + rollback plan
- First action types (all reversible): WAF custom rules (create/update/disable), cache rules, rate-limiting rules, DNS record *edits* (no deletes)
- Approval workflow: review UI, approve/reject with audit trail, per-zone autonomy level (L0–L2)
- Executor: precondition re-check, provider dry-run where supported, apply, post-apply verification window, one-click + automatic rollback
- Action audit log: full chain (evidence → recommendation → approver → result)
- Token/cost accounting per LLM call; per-workspace budgets with hard stop

Out of scope: L3 autonomy, additional providers.

**Done when:** a WAF rule recommended by Korugan is approved, applied, verified live, then rolled back — all from the UI, all visible in the audit log.

---

## P3 — Multi-provider

**Goal:** prove the connector abstraction with the second and third providers; stabilize the connector SDK.

In scope:

- Connector conformance test suite (public, runnable by contributors)
- AWS CloudFront connector; Fastly or BunnyCDN next (community interest decides order)
- Cross-provider views: unified event explorer, per-provider capability degradation (a provider without WAF write simply lacks those recommendations)
- Multi-user: roles (admin / operator / viewer), workspace concept

**Done when:** the same finding→recommendation→apply loop runs on a non-Cloudflare provider with zero domain-code changes — connector code only.

---

## P4 — Scale & autonomy (L3)

**Goal:** earn autonomy; adopt heavy infrastructure only where P1–P3 metrics demand it.

In scope:

- Policy engine for L3: action-type allowlists, scope + rate limits, quiet hours, mandatory reversibility, kill switch
- L3 rollout: per-(zone, action-type) opt-in, gated on L2 track record
- ClickHouse for event analytics **if** volume triggers hit (see ARCHITECTURE.md); NATS **if** a second service ships
- OpenTelemetry maturity: exemplar dashboards, SLOs for connector freshness and action latency
- Kubernetes deployment manifests

**Done when:** an L3 policy handles a real recurring incident class end-to-end (detect → apply → verify) with rollback never manually invoked, across ≥ 30 days.

---

## P5 — Self-hosted edge (agent wave)

**Goal:** extend beyond SaaS edges to infrastructure users run themselves.

In scope:

- Lightweight agent (single Go binary) for Nginx / Traefik / HAProxy / Kubernetes Ingress: log shipping + config templating + reload orchestration
- Agent ↔ core over NATS; agents are the canonical "second service" that justified it
- Akamai / Azure Front Door connectors (enterprise wave)

**Done when:** an Nginx server managed by the agent gets the same finding→fix loop as a Cloudflare zone.

---

## Continuously (every phase)

- Docs written/updated with the code they describe (API.md, DATABASE.md, TESTING.md, DEPLOYMENT.md, SECURITY.md land in P1–P2 as their subjects materialize)
- Security posture: threat model updates, dependency scanning, secret-handling review
- Honest metrics in the README: what works, what's flaky, what's vaporware
