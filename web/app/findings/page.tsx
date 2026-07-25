"use client";

import { Warning } from "@phosphor-icons/react";
import { api, type Finding, type Severity } from "@/lib/api";
import { useApi, AsyncView } from "@/components/data";
import { SectionHeader, Card, SeverityBadge, Tag, EmptyState } from "@/components/ui";
import { timeAgo } from "@/lib/format";

const sevRank: Record<Severity, number> = { critical: 4, high: 3, medium: 2, low: 1, info: 0 };

export default function FindingsPage() {
  const findings = useApi(() => api.findings("open"), []);

  return (
    <div>
      <SectionHeader
        title="Findings"
        subtitle="Detected issues, rule-based and AI-produced, each grounded in real events."
      />
      <AsyncView state={findings} skeletonRows={6}>
        {(d) => {
          const rows = (d.findings ?? [])
            .slice()
            .sort((a, b) => sevRank[b.severity] - sevRank[a.severity]);
          if (rows.length === 0)
            return (
              <EmptyState
                icon={<Warning size={32} weight="duotone" />}
                title="Nothing open"
                body="No open findings right now. Rule-based analyzers run continuously; AI findings appear when a key is configured."
              />
            );
          return (
            <div className="grid grid-cols-1 gap-3">
              {rows.map((f: Finding) => (
                <Card key={f.id} className="p-4">
                  <div className="flex items-start justify-between gap-4">
                    <div className="min-w-0">
                      <div className="flex items-center gap-2">
                        <SeverityBadge severity={f.severity} />
                        <Tag>{f.kind}</Tag>
                        <span className="text-xs text-slate-500">{f.source}</span>
                      </div>
                      <h3 className="mt-2 text-sm font-medium text-slate-100">{f.title}</h3>
                      <p className="mt-1 text-sm text-slate-400">{f.detail}</p>
                    </div>
                    <div className="shrink-0 text-right text-xs text-slate-500">
                      <div>{f.resource.name}</div>
                      <div className="mt-1">{timeAgo(f.updated_at)}</div>
                    </div>
                  </div>
                  {f.evidence?.length ? (
                    <div className="mt-3 border-t border-line pt-3 text-xs text-slate-500">
                      {f.evidence.length} event{f.evidence.length === 1 ? "" : "s"} cited as evidence
                    </div>
                  ) : null}
                </Card>
              ))}
            </div>
          );
        }}
      </AsyncView>
    </div>
  );
}
