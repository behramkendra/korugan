"use client";

import { api } from "@/lib/api";
import { useApi } from "@/components/data";

// Live backend + AI state in the header. Degrades to a clear "offline"
// pill instead of throwing when the API is down.
export function HealthPill() {
  const { data, error } = useApi(() => api.health(), []);

  if (error) {
    return (
      <span className="inline-flex items-center gap-2 rounded-lg border border-rose-500/30 bg-rose-500/10 px-3 py-1.5 text-xs text-rose-300">
        <span className="h-1.5 w-1.5 rounded-full bg-rose-400" />
        API offline
      </span>
    );
  }
  if (!data) {
    return (
      <span className="inline-flex items-center gap-2 rounded-lg border border-line bg-surface-2 px-3 py-1.5 text-xs text-slate-500">
        <span className="h-1.5 w-1.5 rounded-full bg-slate-600" />
        connecting
      </span>
    );
  }
  return (
    <span className="inline-flex items-center gap-3 rounded-lg border border-line bg-surface-2 px-3 py-1.5 text-xs">
      <span className="inline-flex items-center gap-2 text-emerald-300">
        <span className="h-1.5 w-1.5 rounded-full bg-emerald-400" />
        API healthy
      </span>
      <span className="text-slate-600">|</span>
      <span className={data.ai_enabled ? "text-accent" : "text-slate-500"}>
        {data.ai_enabled ? "AI enabled" : "zero-key mode"}
      </span>
    </span>
  );
}
