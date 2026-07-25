# Seed GitHub labels, milestones and starter issues for Korugan (Windows/PowerShell).
#
# Prerequisites:
#   1. The repo exists on GitHub and your local main is pushed.
#   2. Authenticate once (this uses YOUR GitHub account, in your browser):
#        & "C:\Program Files\GitHub CLI\gh.exe" auth login
#
# Run:
#        .\scripts\seed-github.ps1
#        .\scripts\seed-github.ps1 -Repo behramkendra/korugan   # explicit repo
#
# Idempotency: labels/milestones are skipped if they already exist; ISSUES are
# NOT — run this once, or you'll create duplicates.

param(
    [string]$Repo = "behramkendra/korugan"
)

$ErrorActionPreference = "Stop"

# --- locate gh.exe (winget install isn't always on PATH) ---
$gh = (Get-Command gh -ErrorAction SilentlyContinue).Source
if (-not $gh) { $gh = "C:\Program Files\GitHub CLI\gh.exe" }
if (-not (Test-Path $gh)) {
    Write-Error "gh.exe not found. Install with: winget install GitHub.cli"
    exit 1
}

# --- confirm auth ---
& $gh auth status 2>$null
if ($LASTEXITCODE -ne 0) {
    Write-Error "Not authenticated. Run:  & `"$gh`" auth login"
    exit 1
}

Write-Host "Seeding $Repo ..." -ForegroundColor Cyan

# --- labels (create if missing) ---
$labels = @(
    @{ name = "connector";     color = "1d76db"; desc = "Provider connector work" },
    @{ name = "ai-engine";     color = "8250df"; desc = "BYOK AI engine" },
    @{ name = "frontend";      color = "0e8a16"; desc = "Next.js web UI" },
    @{ name = "security";      color = "b60205"; desc = "Security / credential handling" },
    @{ name = "observability"; color = "fbca04"; desc = "Telemetry, metrics, tracing" },
    @{ name = "design-review"; color = "5319e7"; desc = "Design feedback wanted" }
)
foreach ($l in $labels) {
    & $gh label create $l.name --repo $Repo --color $l.color --description $l.desc 2>$null
    if ($LASTEXITCODE -eq 0) { Write-Host "label: $($l.name)" } else { Write-Host "label exists/skip: $($l.name)" }
}

# --- milestones ---
function New-Milestone($title, $desc) {
    & $gh api "repos/$Repo/milestones" -f title="$title" -f description="$desc" 2>$null | Out-Null
    if ($LASTEXITCODE -eq 0) { Write-Host "milestone: $title" } else { Write-Host "milestone exists/skip: $title" }
}
New-Milestone "P1 - Cloudflare read-only MVP"      "Connect a Cloudflare account and get grounded explanations of real traffic."
New-Milestone "P2 - Recommend & approved apply"    "Finding -> recommendation -> approve -> apply -> verify -> rollback."
New-Milestone "P3 - Multi-provider"                "Prove the connector abstraction with a second and third provider."
New-Milestone "P4 - Scale & autonomy (L3)"         "Policy engine, ClickHouse/NATS when metrics demand, L3 opt-in."
New-Milestone "P5 - Self-hosted edge (agents)"     "Nginx/Traefik/HAProxy/K8s Ingress via a lightweight agent."

# --- issues ---
function New-Issue($title, $body, $labels, $milestone) {
    & $gh issue create --repo $Repo --title $title --body $body --label $labels --milestone $milestone | Out-Null
    if ($LASTEXITCODE -eq 0) { Write-Host "issue: $title" } else { Write-Host "issue FAILED: $title" }
}

New-Issue "Web UI: onboarding, zone overview, event explorer, findings, chat" `
"Build the Next.js UI (Tailwind + shadcn/ui) against the existing REST API (docs/API.md). First screens: connect Cloudflare token, zone overview, event explorer with filters, findings feed, grounded chat." `
"frontend,enhancement" "P1 - Cloudflare read-only MVP"

New-Issue "Cloudflare: cache-rules and rate-limit-rules write paths" `
"Extend the Cloudflare writer beyond WAF custom rules and cache.purge to cache rules and rate-limiting rules via the Rulesets API, with rollback material and fixture tests." `
"connector,enhancement" "P2 - Recommend & approved apply"

New-Issue "SSE streaming for chat responses" `
"Stream AI chat token-by-token over Server-Sent Events instead of a single JSON response. Update /api/v1/chat and the engine to support a streaming callback." `
"ai-engine,enhancement" "P1 - Cloudflare read-only MVP"

New-Issue "Sealed credential storage + settings API (replace env-only tokens)" `
"Add a settings API that stores connector and LLM credentials sealed (internal/crypto) in the secrets table, masked in responses, with a two-token pattern (read token at onboarding, write token only when enabling L2)." `
"security,enhancement" "P1 - Cloudflare read-only MVP"

New-Issue "AWS CloudFront connector (design review first)" `
"Map CloudFront onto docs/CONNECTORS.md: distributions as resources, AWS WAF, logs/metrics for events. Open with a design-review issue identifying where the normalized schema strains before writing code." `
"connector,design-review" "P3 - Multi-provider"

New-Issue "Connector conformance test suite" `
"A public, runnable suite every connector must pass: interface contract, event normalization golden files, cursor resume, idempotent apply under retry, rollback round-trip per action type, credential redaction." `
"connector,enhancement" "P3 - Multi-provider"

New-Issue "Policy engine for L3 autonomy" `
"Implement the Policy type end to end: action-type allowlist, per-resource rate limits, quiet hours, mandatory reversibility, kill switch. Gate L3 execution on a matching enabled policy plus an L2 track record." `
"enhancement" "P4 - Scale & autonomy (L3)"

New-Issue "OpenTelemetry traces + metrics wiring" `
"Flesh out internal/obs with real OTLP export: spans for every connector call, LLM call (model/tokens/latency/cost) and action execution; basic dashboards and SLOs." `
"observability,enhancement" "P4 - Scale & autonomy (L3)"

New-Issue "Model price table + per-workspace budget dashboard" `
"Ship an editable model price table (USD/1k tokens) and a usage/budget view backed by llm_usage, so BYOK users see spend per provider/model/task-class and can set caps in the UI." `
"ai-engine,enhancement" "P2 - Recommend & approved apply"

Write-Host "Done." -ForegroundColor Green
