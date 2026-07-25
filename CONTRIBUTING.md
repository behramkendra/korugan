# Contributing to Korugan

Thanks for considering it. The project is early — that makes contributions higher-leverage, not lower.

## Right now: design review is the contribution

We are in P0 (see [ROADMAP.md](./ROADMAP.md)): documentation exists, code is landing next. The most valuable things you can do today:

- **Attack the connector spec** ([docs/CONNECTORS.md](./docs/CONNECTORS.md)) — especially if you know CloudFront, Fastly, Akamai or Bunny internals. Does the interface survive your provider?
- **Attack the event/action schemas** — a category or field that won't generalize is much cheaper to fix now.
- **Challenge decisions** in the [decision log](./docs/ARCHITECTURE.md#decision-log) — with reasoning, in an issue.
- Typos/clarity PRs are welcome, no issue needed.

Open an issue with the `design-review` label. Disagreement with specifics beats praise with vagueness.

## Development setup (applies from P1)

Prerequisites: **Go 1.26+**, **Node 22+**, **PostgreSQL 16+**.

Standard path (Docker):

```bash
git clone https://github.com/behramkendra/korugan.git
cd korugan
docker compose up -d db        # Postgres with dev defaults
make dev                       # runs migrations, starts API with live reload
cd web && npm install && npm run dev
```

No Docker? Any reachable Postgres works:

```bash
export DATABASE_URL=postgres://user:pass@host:5432/korugan?sslmode=disable
make dev
```

LLM features are optional in development — the zero-key mode is a supported state, not a degraded one. If you want them, add your own key (OpenRouter/Anthropic/OpenAI/DeepSeek/Ollama) via the UI or `KORUGAN_LLM_*` env vars. **Never commit keys; never paste keys into issues.** `.env*` is gitignored on purpose.

## Code expectations

- `gofmt` + `golangci-lint run` clean; frontend passes `npm run lint`.
- Tests accompany behavior: table-driven unit tests for domain logic, golden-file tests for connector normalization (sanitized fixtures — **no live API calls in CI**).
- Keep module boundaries (see [docs/ARCHITECTURE.md](./docs/ARCHITECTURE.md#module-layout)): `domain` imports nothing internal; cross-module access goes through interfaces.
- Small PRs. A connector arrives as a series (interface stub → read paths → events → actions), not one 4,000-line drop.

## Commits & PRs

- **Conventional Commits:** `feat(connector): add fastly event paging`, `fix(ai): redact keys in provider errors`, `docs: clarify autonomy gates`.
- **DCO:** sign off every commit (`git commit -s`). This certifies the [Developer Certificate of Origin](https://developercertificate.org/); no CLA.
- PR description says *why*, links the issue, and states how you verified it.
- CI (lint + tests) must be green; a maintainer review merges it.

## Issue labels

`design-review` · `bug` · `connector` · `ai-engine` · `frontend` · `docs` · `good-first-issue` · `help-wanted`

## Security issues

**Do not open public issues for vulnerabilities.** Use GitHub's private vulnerability reporting ("Report a vulnerability" under the Security tab). Credential handling, injection paths, and the action pipeline are the crown jewels — reports there are treated as top priority.

## Conduct

Be kind, be direct, argue about ideas not people. Maintainers may remove contributions and contributors that don't manage this. A formal code of conduct document will be adopted before the first release.

## License

By contributing you agree your contributions are licensed under [Apache-2.0](./LICENSE), per the DCO you sign.
