"use client";

import { useState } from "react";
import { Lightbulb, ArrowUUpLeft, Check, X } from "@phosphor-icons/react";
import { api, type Recommendation, ApiError } from "@/lib/api";
import { useApi, AsyncView } from "@/components/data";
import { SectionHeader, Card, Tag, Button, EmptyState } from "@/components/ui";
import { timeAgo } from "@/lib/format";

type Outcome = { kind: "ok" | "err"; text: string };

export default function RecommendationsPage() {
  const recs = useApi(() => api.recommendations(), []);
  const [busy, setBusy] = useState<string | null>(null);
  const [outcomes, setOutcomes] = useState<Record<string, Outcome>>({});

  // The approver identity is a placeholder until session auth lands; the
  // backend records whatever actor we send in the audit log.
  const actor = "operator";

  async function act(id: string, kind: "approve" | "reject") {
    setBusy(id);
    try {
      if (kind === "approve") {
        const r = await api.approve(id, actor);
        setOutcomes((o) => ({ ...o, [id]: { kind: "ok", text: `Applied. Action ${r.action_id.slice(0, 8)} ${r.status}.` } }));
      } else {
        await api.reject(id, actor, "rejected from console");
        setOutcomes((o) => ({ ...o, [id]: { kind: "ok", text: "Rejected and recorded." } }));
      }
      recs.reload();
    } catch (e) {
      const msg = e instanceof ApiError ? e.message : String(e);
      setOutcomes((o) => ({ ...o, [id]: { kind: "err", text: msg } }));
    } finally {
      setBusy(null);
    }
  }

  return (
    <div>
      <SectionHeader
        title="Recommendations"
        subtitle="Provider-ready changes proposed for a finding. Nothing is applied without your approval."
      />
      <AsyncView state={recs} skeletonRows={4}>
        {(d) => {
          const rows = d.recommendations ?? [];
          if (rows.length === 0)
            return (
              <EmptyState
                icon={<Lightbulb size={32} weight="duotone" />}
                title="No recommendations"
                body="When the AI engine turns a finding into a concrete fix, it shows up here with a diff and a rollback plan."
              />
            );
          return (
            <div className="grid grid-cols-1 gap-4">
              {rows.map((r: Recommendation) => {
                const outcome = outcomes[r.id];
                return (
                  <Card key={r.id} className="p-5">
                    <div className="flex flex-wrap items-center gap-2">
                      <Tag>{r.action_type}</Tag>
                      <span className="text-xs text-slate-500">{r.resource.name}</span>
                      <span className="text-xs text-slate-600">· {timeAgo(r.created_at)}</span>
                      <span className="ml-auto text-xs text-slate-500">
                        confidence {Math.round(r.confidence * 100)}%
                      </span>
                    </div>

                    <p className="mt-3 text-sm text-slate-200">{r.rationale}</p>

                    {Object.keys(r.params ?? {}).length > 0 ? (
                      <pre className="mt-3 overflow-x-auto rounded-lg border border-line bg-bg p-3 font-mono text-xs text-slate-300">
                        {JSON.stringify(r.params, null, 2)}
                      </pre>
                    ) : null}

                    <div className="mt-3 flex items-start gap-2 rounded-lg border border-line bg-surface-2 p-3 text-xs text-slate-400">
                      <ArrowUUpLeft size={14} className="mt-0.5 shrink-0 text-slate-500" />
                      <span>
                        <span className="text-slate-300">Rollback:</span> {r.rollback_plan}
                      </span>
                    </div>

                    {outcome ? (
                      <div
                        className={`mt-3 rounded-lg border p-2 text-xs ${
                          outcome.kind === "ok"
                            ? "border-emerald-500/30 bg-emerald-500/10 text-emerald-300"
                            : "border-rose-500/30 bg-rose-500/10 text-rose-300"
                        }`}
                      >
                        {outcome.text}
                      </div>
                    ) : (
                      <div className="mt-4 flex gap-2">
                        <Button onClick={() => act(r.id, "approve")} disabled={busy === r.id}>
                          <Check size={16} /> {busy === r.id ? "Applying..." : "Approve and apply"}
                        </Button>
                        <Button variant="danger" onClick={() => act(r.id, "reject")} disabled={busy === r.id}>
                          <X size={16} /> Reject
                        </Button>
                      </div>
                    )}
                  </Card>
                );
              })}
            </div>
          );
        }}
      </AsyncView>
    </div>
  );
}
