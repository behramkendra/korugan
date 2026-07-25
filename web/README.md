# Korugan Web

The Next.js console for [Korugan](../README.md). Talks to the Go API over REST.

## Develop

```bash
cp .env.example .env.local     # point NEXT_PUBLIC_API_BASE at your backend
npm install
npm run dev                    # http://localhost:3000
```

The backend must be running (see the root README). With no LLM key configured
the backend runs in zero-key mode: every screen works except AI chat, which
shows a clear prompt to add a key.

## Screens

- **Dashboard** - resources, open findings by severity, pending recommendations
- **Events** - normalized event explorer with category filters
- **Findings** - rule-based and AI findings with evidence counts
- **Recommendations** - approve (apply + verify + rollback) or reject, per the autonomy ladder
- **Chat** - grounded questions over your own events and findings

## Stack

Next.js (App Router) · TypeScript · Tailwind CSS · Phosphor icons. Dark-locked
theme matching the Korugan palette. Every screen implements real loading,
empty and error states, and degrades cleanly when the backend is offline.
