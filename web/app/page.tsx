"use client";

import Link from "next/link";
import { Globe, Warning, Lightbulb, PlugsConnected } from "@phosphor-icons/react";
import { api, type Finding, type Severity } from "@/lib/api";
import { useApi, AsyncView } from "@/components/data";
import { SectionHeader, StatTile, Card, SeverityBadge, EmptyState, Tag } from "@/components/ui";
import { timeAgo } from "@/lib/format";

const sevRank: Record<Severity, number> = { critical: 4, high: 3, medium: 2, low: 1, info: 0 };

export default function Dashboard() {
  const resources = useApi(() => api.resources(), []);
  const findings = useApi(() => api.findings("open"), []);
  const recs = useApi(() => api.recommendations(), []);

  const resList = resources.data?.resources ?? [];
  const findList = (findings.data?.findings ?? []).slice().sort(
    (a, b) => sevRank[b.severity] - sevRank[a.severity],
  );
  const recCount = recs.data?.recommendations?.length ?? 0;
  const highCount = findList.filter((f) => f.severity === "high" || f.severity === "critical").length;

  return (
    <div>
      <SectionHeader
        title="Overview"
        subtitle="Connected edge resources, open findings, and pending recommendations."
      />

      <div className="grid grid-cols-2 gap-4 lg:grid-cols-4">
        <StatTile label="Resources" value={resList.length} tone="accent" hint="synced zones" />
        <StatTile label="Open findings" value={findList.length} hint="rule + AI detected" />
        <StatTile
          label="High severity"
          value={highCount}
          tone={highCount > 0 ? "danger" : "default"}
          hint="needs attention"
        />
        <StatTile
          label="Recommendations"
          value={recCount}
          tone={recCount > 0 ? "warn" : "default"}
          hint="awaiting review"
        />
      </div>

      <div className="mt-8 grid grid-cols-1 gap-6 lg:grid-cols-3">
        <div className="lg:col-span-2">
          <div className="mb-3 flex items-center justify-between">
            <h2 className="flex items-center gap-2 text-sm font-medium text-slate-200">
              <Warning size={16} className="text-amber-400" /> Top open findings
            </h2>
            <Link href="/findings" className="text-xs text-accent hover:underline">
              View all
            </Link>
          </div>
          <AsyncView state={findings} skeletonRows={4}>
            {(d) => {
              const items = (d.findings ?? [])
                .slice()
                .sort((a, b) => sevRank[b.severity] - sevRank[a.severity])
                .slice(0, 5);
              if (items.length === 0)
                return (
                  <EmptyState
                    icon={<Warning size={32} weight="duotone" />}
                    title="No open findings"
                    body="Once a Cloudflare token is connected and traffic is analyzed, findings appear here."
                  />
                );
              return (
                <Card className="divide-y divide-line">
                  {items.map((f: Finding) => (
                    <div key={f.id} className="flex items-start gap-3 p-4">
                      <SeverityBadge severity={f.severity} />
                      <div className="min-w-0 flex-1">
                        <div className="truncate text-sm text-slate-200">{f.title}</div>
                        <div className="mt-0.5 truncate text-xs text-slate-500">
                          {f.resource.name} · {timeAgo(f.updated_at)}
                        </div>
                      </div>
                      <Tag>{f.source}</Tag>
                    </div>
                  ))}
                </Card>
              );
            }}
          </AsyncView>
        </div>

        <div>
          <h2 className="mb-3 flex items-center gap-2 text-sm font-medium text-slate-200">
            <Globe size={16} className="text-accent" /> Resources
          </h2>
          <AsyncView state={resources} skeletonRows={4}>
            {(d) => {
              const items = d.resources ?? [];
              if (items.length === 0)
                return (
                  <EmptyState
                    icon={<PlugsConnected size={32} weight="duotone" />}
                    title="No resources yet"
                    body="Set CLOUDFLARE_API_TOKEN and the sync loop will pull your zones."
                  />
                );
              return (
                <Card className="divide-y divide-line">
                  {items.slice(0, 8).map((r) => (
                    <div key={r.id} className="flex items-center gap-3 p-3">
                      <Globe size={16} className="text-slate-500" />
                      <div className="min-w-0 flex-1">
                        <div className="truncate text-sm text-slate-200">{r.ref.name}</div>
                        <div className="text-xs text-slate-500">{r.ref.provider}</div>
                      </div>
                    </div>
                  ))}
                </Card>
              );
            }}
          </AsyncView>

          <h2 className="mb-3 mt-6 flex items-center gap-2 text-sm font-medium text-slate-200">
            <Lightbulb size={16} className="text-amber-400" /> Pending review
          </h2>
          <Card className="p-4">
            <div className="font-mono text-3xl font-semibold text-slate-100">{recCount}</div>
            <p className="mt-1 text-xs text-slate-500">
              recommendations waiting for approval.
            </p>
            <Link
              href="/recommendations"
              className="mt-3 inline-block text-xs text-accent hover:underline"
            >
              Review recommendations
            </Link>
          </Card>
        </div>
      </div>
    </div>
  );
}
