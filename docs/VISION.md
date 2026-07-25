# Vision

## The problem

Modern applications live behind edge platforms — Cloudflare, CloudFront, Fastly, Akamai — that answer millions of requests and absorb attacks around the clock. Operating them well has three chronic problems:

**1. Dashboards show data, not decisions.**
Every provider ships analytics: request graphs, threat scores, cache ratios, firewall event tables. None of them answer "so what should I do?" The gap between *seeing* a spike in blocked traffic and *knowing* whether it's a credential-stuffing attack, a broken client, or a false-positive WAF rule is filled today by human expertise — when it exists.

**2. Edge expertise is scarce and provider-specific.**
Writing a correct WAF custom rule, tuning cache keys, or diagnosing an origin handshake failure requires knowledge most teams don't have in-house. The engineer who knows Cloudflare's rulesets language rarely also knows Akamai's. Small teams run on defaults; defaults leak money and security.

**3. Alert fatigue without action.**
Monitoring tools generate notifications; humans triage them; most are noise; the real ones arrive at 03:00. The loop from signal → diagnosis → fix → verification is entirely manual, and it doesn't scale with the number of zones, providers, and services a team operates.

## The thesis

**The edge control plane should be operated conversationally, and — with consent — autonomously.**

Large language models are now good enough to do real operations work when they are grounded in real data and constrained by real guardrails: read the same events a human would, explain causes, draft the exact provider configuration change, predict its blast radius, and monitor the result. What they need is scaffolding — normalized data, safe tools, an approval workflow, an audit trail. Korugan is that scaffolding.

The long-term shape: an **edge security operating system** — a self-hosted layer that treats heterogeneous providers as devices, normalized events as its bus, and an AI engine as its scheduler, with the human operator as root.

## Principles

1. **Self-host first.** Your traffic metadata and security events are sensitive. Korugan runs in your infrastructure; nothing is phoned home.
2. **BYOK — your models, your bill.** Korugan never resells inference. Users bring their own API keys (Anthropic, OpenAI, DeepSeek, OpenRouter) or run local models (Ollama). The project has zero inference cost by design, which is what makes it sustainable as open source.
3. **Trust is earned in levels.** Read-only by default. Recommendations before actions. Approvals before autonomy. Autonomy only within narrow, reversible, per-zone policies. No exceptions, including for demos.
4. **Explain like an engineer, not a dashboard.** Output is causal ("blocked traffic tripled because rule 4711 started matching Googlebot after Tuesday's change"), not descriptive ("blocked requests: +212%").
5. **The connector is the contract.** Providers differ wildly; the normalized event/action schema is the project's most carefully guarded API. A feature that can't be expressed provider-neutrally belongs in a provider extension, not the core.
6. **Boring infrastructure until metrics say otherwise.** Go and Postgres until event volume, service count, or query latency force an upgrade. Ambition goes into the product, not the deployment diagram.

## What Korugan is not

- **Not a CDN or WAF.** Korugan manages edge platforms; it is not in the request path and adds zero latency to your traffic.
- **Not a SaaS (yet).** V1 is self-hosted OSS. A managed offering may exist someday; it will never be required.
- **Not a SIEM replacement.** Korugan is deep on edge/CDN signals, not a general log lake for every system you run.
- **Not a model company.** No training, no fine-tuning, no proprietary models. Orchestration, grounding and guardrails are the product.
- **Not autonomous-by-default.** An AI that silently edits production DNS is a threat, not a feature. Autonomy is the last milestone, not the first.

## Expansion path

| Wave | Providers | Why this order |
|---|---|---|
| 1 | **Cloudflare** | Richest API surface, best documentation, largest self-hosting community overlap |
| 2 | AWS CloudFront, Fastly, BunnyCDN, Vercel | Well-documented APIs, high demand, moderate integration cost |
| 3 | Akamai, Azure Front Door | Enterprise reach, significantly heavier APIs |
| 4 | Nginx, Traefik, HAProxy, Kubernetes Ingress | Self-hosted edge via lightweight agent — different mechanics (log shipping + config templating), same schemas |

Each wave must not require changes to the connector interface. If it does, the interface was wrong and gets fixed first.

## North star

> An operator asks one question — or asks nothing at all — instead of spending an afternoon across four dashboards.

When a mid-sized team can run multi-provider edge infrastructure at the quality of a company with a dedicated edge security team, Korugan is working.
