"use client";

import { useState } from "react";
import { ListBullets } from "@phosphor-icons/react";
import { api, type EventRow } from "@/lib/api";
import { useApi, AsyncView } from "@/components/data";
import { SectionHeader, Card, SeverityBadge, Tag, EmptyState } from "@/components/ui";
import { timeAgo } from "@/lib/format";

const categories = [
  "",
  "waf.block",
  "waf.challenge",
  "ratelimit.hit",
  "bot.detected",
  "origin.error",
  "ssl.cert_expiring",
];

export default function EventsPage() {
  const [category, setCategory] = useState("");
  const events = useApi(() => api.events({ category: category || undefined, limit: 150 }), [category]);

  return (
    <div>
      <SectionHeader
        title="Event explorer"
        subtitle="Normalized security and traffic events from every connected provider."
      />

      <div className="mb-4 flex flex-wrap gap-2">
        {categories.map((c) => (
          <button
            key={c || "all"}
            onClick={() => setCategory(c)}
            className={`rounded-lg border px-3 py-1.5 text-xs transition ${
              category === c
                ? "border-accent/40 bg-accent/10 text-accent"
                : "border-line text-slate-400 hover:bg-surface-2"
            }`}
          >
            {c === "" ? "All" : c}
          </button>
        ))}
      </div>

      <AsyncView state={events} skeletonRows={8}>
        {(d) => {
          const rows = d.events ?? [];
          if (rows.length === 0)
            return (
              <EmptyState
                icon={<ListBullets size={32} weight="duotone" />}
                title="No events in this view"
                body="Events populate as the connector polls your providers. Try a different category or connect a token."
              />
            );
          return (
            <Card className="overflow-hidden">
              <div className="overflow-x-auto">
                <table className="w-full text-left text-sm">
                  <thead className="border-b border-line text-xs uppercase tracking-wide text-slate-500">
                    <tr>
                      <th className="px-4 py-3 font-medium">Severity</th>
                      <th className="px-4 py-3 font-medium">Category</th>
                      <th className="px-4 py-3 font-medium">Target</th>
                      <th className="px-4 py-3 font-medium">Source</th>
                      <th className="px-4 py-3 font-medium">When</th>
                    </tr>
                  </thead>
                  <tbody className="divide-y divide-line">
                    {rows.map((e: EventRow) => (
                      <tr key={e.id} className="hover:bg-surface-2/50">
                        <td className="px-4 py-3">
                          <SeverityBadge severity={e.severity} />
                        </td>
                        <td className="px-4 py-3">
                          <Tag>{e.category}</Tag>
                        </td>
                        <td className="px-4 py-3 text-slate-300">
                          <span className="font-mono text-xs">
                            {e.target?.method ? `${e.target.method} ` : ""}
                            {e.target?.path ?? e.target?.host ?? "-"}
                          </span>
                          {e.actor?.country ? (
                            <span className="ml-2 text-xs text-slate-500">{e.actor.country}</span>
                          ) : null}
                        </td>
                        <td className="px-4 py-3 text-xs text-slate-400">{e.resource.name}</td>
                        <td className="px-4 py-3 text-xs text-slate-500">{timeAgo(e.ts)}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </Card>
          );
        }}
      </AsyncView>
    </div>
  );
}
