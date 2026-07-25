import type { ReactNode } from "react";
import type { Severity } from "@/lib/api";

// Shared primitives. One radius scale (rounded-lg), one accent (cyan),
// severity colors mapped once and reused everywhere.

export function Card({ children, className = "" }: { children: ReactNode; className?: string }) {
  return (
    <div className={`rounded-lg border border-line bg-surface ${className}`}>{children}</div>
  );
}

export function SectionHeader({ title, subtitle }: { title: string; subtitle?: string }) {
  return (
    <div className="mb-5">
      <h1 className="text-xl font-semibold tracking-tight text-slate-100">{title}</h1>
      {subtitle ? <p className="mt-1 text-sm text-slate-400">{subtitle}</p> : null}
    </div>
  );
}

const sevStyles: Record<Severity, string> = {
  critical: "bg-rose-500/15 text-rose-300 border-rose-500/30",
  high: "bg-amber-500/15 text-amber-300 border-amber-500/30",
  medium: "bg-yellow-500/15 text-yellow-200 border-yellow-500/30",
  low: "bg-sky-500/15 text-sky-300 border-sky-500/30",
  info: "bg-slate-500/15 text-slate-300 border-slate-500/30",
};

export function SeverityBadge({ severity }: { severity: Severity }) {
  return (
    <span
      className={`inline-flex items-center rounded-lg border px-2 py-0.5 text-xs font-medium capitalize ${sevStyles[severity] ?? sevStyles.info}`}
    >
      {severity}
    </span>
  );
}

export function Tag({ children }: { children: ReactNode }) {
  return (
    <span className="inline-flex items-center rounded-lg border border-line bg-surface-2 px-2 py-0.5 font-mono text-xs text-slate-300">
      {children}
    </span>
  );
}

export function StatTile({
  label,
  value,
  hint,
  tone = "default",
}: {
  label: string;
  value: ReactNode;
  hint?: string;
  tone?: "default" | "accent" | "warn" | "danger";
}) {
  const toneClass =
    tone === "accent"
      ? "text-accent"
      : tone === "warn"
        ? "text-amber-300"
        : tone === "danger"
          ? "text-rose-300"
          : "text-slate-100";
  return (
    <Card className="p-4">
      <div className="text-xs uppercase tracking-wide text-slate-500">{label}</div>
      <div className={`mt-2 font-mono text-3xl font-semibold ${toneClass}`}>{value}</div>
      {hint ? <div className="mt-1 text-xs text-slate-500">{hint}</div> : null}
    </Card>
  );
}

export function Button({
  children,
  onClick,
  variant = "primary",
  disabled = false,
  type = "button",
}: {
  children: ReactNode;
  onClick?: () => void;
  variant?: "primary" | "ghost" | "danger";
  disabled?: boolean;
  type?: "button" | "submit";
}) {
  const base =
    "inline-flex items-center justify-center gap-2 rounded-lg px-3 py-2 text-sm font-medium transition active:translate-y-px disabled:cursor-not-allowed disabled:opacity-50";
  const styles =
    variant === "primary"
      ? "bg-accent text-slate-950 hover:bg-cyan-300"
      : variant === "danger"
        ? "border border-rose-500/40 text-rose-300 hover:bg-rose-500/10"
        : "border border-line text-slate-200 hover:bg-surface-2";
  return (
    <button type={type} onClick={onClick} disabled={disabled} className={`${base} ${styles}`}>
      {children}
    </button>
  );
}

export function EmptyState({
  icon,
  title,
  body,
}: {
  icon?: ReactNode;
  title: string;
  body: string;
}) {
  return (
    <Card className="flex flex-col items-center justify-center px-6 py-16 text-center">
      {icon ? <div className="mb-3 text-slate-600">{icon}</div> : null}
      <div className="text-sm font-medium text-slate-300">{title}</div>
      <div className="mt-1 max-w-sm text-sm text-slate-500">{body}</div>
    </Card>
  );
}

export function Skeleton({ rows = 5 }: { rows?: number }) {
  return (
    <Card className="divide-y divide-line">
      {Array.from({ length: rows }).map((_, i) => (
        <div key={i} className="flex items-center gap-4 p-4">
          <div className="h-4 w-24 animate-pulse rounded bg-surface-2" />
          <div className="h-4 flex-1 animate-pulse rounded bg-surface-2" />
          <div className="h-4 w-16 animate-pulse rounded bg-surface-2" />
        </div>
      ))}
    </Card>
  );
}
