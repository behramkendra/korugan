// Typed client for the Korugan Go API. Base URL and optional bearer token
// come from environment; the browser talks to the self-hosted backend.

const BASE = process.env.NEXT_PUBLIC_API_BASE ?? "http://localhost:8080";
const TOKEN = process.env.NEXT_PUBLIC_API_TOKEN ?? "";

export type ResourceRef = {
  provider: string;
  kind: string;
  external_id: string;
  name: string;
};

export type Health = { ok: boolean; ai_enabled: boolean };

export type ResourceRow = { id: string; ref: ResourceRef; created_at: string };

export type Severity = "info" | "low" | "medium" | "high" | "critical";

export type EventRow = {
  id: string;
  provider_event_id: string;
  resource: ResourceRef;
  category: string;
  severity: Severity;
  ts: string;
  actor?: { ip?: string; country?: string; asn?: number; user_agent?: string };
  target?: { host?: string; path?: string; method?: string };
  rule?: { id?: string; name?: string; action_taken?: string };
};

export type Finding = {
  id: string;
  resource: ResourceRef;
  kind: string;
  severity: Severity;
  state: string;
  title: string;
  detail: string;
  evidence: string[];
  source: string;
  created_at: string;
  updated_at: string;
};

export type Recommendation = {
  id: string;
  finding_id: string;
  resource: ResourceRef;
  action_type: string;
  params: Record<string, unknown>;
  rationale: string;
  rollback_plan: string;
  confidence: number;
  created_at: string;
};

export type ActionRow = {
  id: string;
  resource: ResourceRef;
  type: string;
  state: string;
  approved_by?: string;
  autonomy_level: number;
  created_at: string;
  updated_at: string;
};

export class ApiError extends Error {
  constructor(
    message: string,
    readonly status: number,
  ) {
    super(message);
  }
}

async function req<T>(path: string, init?: RequestInit): Promise<T> {
  const headers: Record<string, string> = { "Content-Type": "application/json" };
  if (TOKEN) headers["Authorization"] = `Bearer ${TOKEN}`;
  let res: Response;
  try {
    res = await fetch(`${BASE}${path}`, { ...init, headers, cache: "no-store" });
  } catch {
    throw new ApiError(`Cannot reach the Korugan API at ${BASE}`, 0);
  }
  if (!res.ok) {
    let msg = `Request failed (${res.status})`;
    try {
      const body = (await res.json()) as { error?: string };
      if (body.error) msg = body.error;
    } catch {
      /* keep default */
    }
    throw new ApiError(msg, res.status);
  }
  return (await res.json()) as T;
}

export const api = {
  health: () => req<Health>("/healthz"),
  resources: () => req<{ resources: ResourceRow[] | null }>("/api/v1/resources"),
  events: (params: { resource_id?: string; category?: string; limit?: number } = {}) => {
    const q = new URLSearchParams();
    if (params.resource_id) q.set("resource_id", params.resource_id);
    if (params.category) q.set("category", params.category);
    q.set("limit", String(params.limit ?? 100));
    return req<{ events: EventRow[] | null }>(`/api/v1/events?${q.toString()}`);
  },
  findings: (state = "open") =>
    req<{ findings: Finding[] | null }>(`/api/v1/findings?state=${state}`),
  recommendations: () =>
    req<{ recommendations: Recommendation[] | null }>("/api/v1/recommendations"),
  approve: (id: string, actor: string) =>
    req<{ action_id: string; status: string }>(
      `/api/v1/recommendations/${id}/approve`,
      { method: "POST", body: JSON.stringify({ actor }) },
    ),
  reject: (id: string, actor: string, reason: string) =>
    req<{ status: string }>(`/api/v1/recommendations/${id}/reject`, {
      method: "POST",
      body: JSON.stringify({ actor, reason }),
    }),
  chat: (message: string, resource_id?: string) =>
    req<{ answer: string; grounded_events: number }>("/api/v1/chat", {
      method: "POST",
      body: JSON.stringify({ message, resource_id }),
    }),
};

export const apiBase = BASE;
