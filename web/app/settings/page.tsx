"use client";

import { useState } from "react";
import { CloudCheck, Key, ShieldCheck, WarningCircle } from "@phosphor-icons/react";
import { api, ApiError, type LLMInput } from "@/lib/api";
import { useApi, AsyncView } from "@/components/data";
import { SectionHeader, Card, Button, Tag } from "@/components/ui";

const providers = ["openrouter", "openai", "deepseek", "anthropic", "ollama"];

type Msg = { kind: "ok" | "err"; text: string } | null;

export default function SettingsPage() {
  const status = useApi(() => api.settings(), []);

  return (
    <div>
      <SectionHeader
        title="Settings"
        subtitle="Store provider and LLM credentials sealed in the database. Keys are AES-256-GCM encrypted and never shown again."
      />
      <AsyncView state={status} skeletonRows={3}>
        {(s) => {
          if (!s.sealed_storage) {
            return (
              <Card className="flex items-start gap-3 p-5">
                <WarningCircle size={22} className="mt-0.5 shrink-0 text-amber-400" weight="duotone" />
                <div>
                  <div className="text-sm font-medium text-slate-200">Sealed storage is off</div>
                  <p className="mt-1 text-sm text-slate-400">
                    Set <code className="rounded bg-bg px-1 py-0.5 font-mono text-xs">KORUGAN_MASTER_KEY</code>{" "}
                    on the server to enable credential storage. Until then, credentials come from
                    environment variables only.
                  </p>
                  <p className="mt-2 font-mono text-xs text-slate-500">openssl rand -base64 32</p>
                </div>
              </Card>
            );
          }
          return (
            <div className="grid grid-cols-1 gap-6 lg:grid-cols-2">
              <CloudflareCard
                configured={s.cloudflare?.configured ?? false}
                hint={s.cloudflare?.hint}
                onSaved={status.reload}
              />
              <LLMCard
                configured={s.llm?.configured ?? false}
                provider={s.llm?.provider}
                model={s.llm?.model}
                hint={s.llm?.key_hint}
                onSaved={status.reload}
              />
            </div>
          );
        }}
      </AsyncView>
    </div>
  );
}

function StatusRow({ configured, children }: { configured: boolean; children: React.ReactNode }) {
  return (
    <div className="mb-4 flex items-center gap-2 text-xs">
      <span
        className={`inline-flex items-center gap-1.5 rounded-lg border px-2 py-1 ${
          configured
            ? "border-emerald-500/30 bg-emerald-500/10 text-emerald-300"
            : "border-line bg-surface-2 text-slate-400"
        }`}
      >
        <span className={`h-1.5 w-1.5 rounded-full ${configured ? "bg-emerald-400" : "bg-slate-600"}`} />
        {configured ? "Configured" : "Not set"}
      </span>
      {children}
    </div>
  );
}

function Notice({ msg }: { msg: Msg }) {
  if (!msg) return null;
  return (
    <div
      className={`mt-3 rounded-lg border p-2 text-xs ${
        msg.kind === "ok"
          ? "border-emerald-500/30 bg-emerald-500/10 text-emerald-300"
          : "border-rose-500/30 bg-rose-500/10 text-rose-300"
      }`}
    >
      {msg.text}
    </div>
  );
}

function CloudflareCard({
  configured,
  hint,
  onSaved,
}: {
  configured: boolean;
  hint?: string;
  onSaved: () => void;
}) {
  const [token, setToken] = useState("");
  const [busy, setBusy] = useState(false);
  const [msg, setMsg] = useState<Msg>(null);

  async function save() {
    setBusy(true);
    setMsg(null);
    try {
      const r = await api.setCloudflare(token);
      setToken("");
      setMsg({ kind: "ok", text: `Saved. ${r.applies}.` });
      onSaved();
    } catch (e) {
      setMsg({ kind: "err", text: e instanceof ApiError ? e.message : String(e) });
    } finally {
      setBusy(false);
    }
  }

  return (
    <Card className="p-5">
      <div className="mb-3 flex items-center gap-2">
        <CloudCheck size={20} className="text-accent" weight="duotone" />
        <h2 className="text-sm font-medium text-slate-100">Cloudflare</h2>
      </div>
      <StatusRow configured={configured}>
        {configured && hint ? <Tag>{hint}</Tag> : null}
      </StatusRow>
      <label className="mb-2 block text-xs text-slate-400" htmlFor="cf-token">
        API token
      </label>
      <input
        id="cf-token"
        type="password"
        value={token}
        onChange={(e) => setToken(e.target.value)}
        placeholder={configured ? "Enter a new token to replace" : "Least-privilege read token"}
        className="w-full rounded-lg border border-line bg-bg px-3 py-2 text-sm text-slate-200 placeholder:text-slate-600 focus:border-accent/50 focus:outline-none"
      />
      <p className="mt-2 text-xs text-slate-500">
        Recommended scopes: Zone Read, DNS Read, Analytics Read, Firewall Services Read.
      </p>
      <div className="mt-4">
        <Button onClick={save} disabled={busy || !token.trim()}>
          <ShieldCheck size={16} /> {busy ? "Sealing..." : "Save sealed"}
        </Button>
      </div>
      <Notice msg={msg} />
    </Card>
  );
}

function LLMCard({
  configured,
  provider,
  model,
  hint,
  onSaved,
}: {
  configured: boolean;
  provider?: string;
  model?: string;
  hint?: string;
  onSaved: () => void;
}) {
  const [form, setForm] = useState<LLMInput>({ provider: "openrouter", model: "", api_key: "", base_url: "" });
  const [busy, setBusy] = useState(false);
  const [msg, setMsg] = useState<Msg>(null);

  const keyOptional = form.provider === "ollama";

  async function save() {
    setBusy(true);
    setMsg(null);
    try {
      const r = await api.setLLM(form);
      setForm((f) => ({ ...f, api_key: "" }));
      setMsg({ kind: "ok", text: `Saved. ${r.applies}.` });
      onSaved();
    } catch (e) {
      setMsg({ kind: "err", text: e instanceof ApiError ? e.message : String(e) });
    } finally {
      setBusy(false);
    }
  }

  return (
    <Card className="p-5">
      <div className="mb-3 flex items-center gap-2">
        <Key size={20} className="text-accent" weight="duotone" />
        <h2 className="text-sm font-medium text-slate-100">LLM (Bring Your Own Key)</h2>
      </div>
      <StatusRow configured={configured}>
        {configured ? (
          <span className="text-slate-400">
            {provider} · {model} · <span className="font-mono">{hint}</span>
          </span>
        ) : null}
      </StatusRow>

      <div className="grid grid-cols-2 gap-3">
        <div>
          <label className="mb-2 block text-xs text-slate-400" htmlFor="llm-provider">
            Provider
          </label>
          <select
            id="llm-provider"
            value={form.provider}
            onChange={(e) => setForm((f) => ({ ...f, provider: e.target.value }))}
            className="w-full rounded-lg border border-line bg-bg px-3 py-2 text-sm text-slate-200 focus:border-accent/50 focus:outline-none"
          >
            {providers.map((p) => (
              <option key={p} value={p}>
                {p}
              </option>
            ))}
          </select>
        </div>
        <div>
          <label className="mb-2 block text-xs text-slate-400" htmlFor="llm-model">
            Model
          </label>
          <input
            id="llm-model"
            value={form.model}
            onChange={(e) => setForm((f) => ({ ...f, model: e.target.value }))}
            placeholder="e.g. anthropic/claude-haiku"
            className="w-full rounded-lg border border-line bg-bg px-3 py-2 text-sm text-slate-200 placeholder:text-slate-600 focus:border-accent/50 focus:outline-none"
          />
        </div>
      </div>

      <label className="mb-2 mt-3 block text-xs text-slate-400" htmlFor="llm-key">
        API key {keyOptional ? <span className="text-slate-600">(optional for Ollama)</span> : null}
      </label>
      <input
        id="llm-key"
        type="password"
        value={form.api_key}
        onChange={(e) => setForm((f) => ({ ...f, api_key: e.target.value }))}
        placeholder={configured ? "Enter a new key to replace" : "Your provider key"}
        className="w-full rounded-lg border border-line bg-bg px-3 py-2 text-sm text-slate-200 placeholder:text-slate-600 focus:border-accent/50 focus:outline-none"
      />

      <label className="mb-2 mt-3 block text-xs text-slate-400" htmlFor="llm-base">
        Base URL <span className="text-slate-600">(optional override)</span>
      </label>
      <input
        id="llm-base"
        value={form.base_url}
        onChange={(e) => setForm((f) => ({ ...f, base_url: e.target.value }))}
        placeholder="Leave empty for the provider default"
        className="w-full rounded-lg border border-line bg-bg px-3 py-2 text-sm text-slate-200 placeholder:text-slate-600 focus:border-accent/50 focus:outline-none"
      />

      <div className="mt-4">
        <Button onClick={save} disabled={busy || !form.model.trim() || (!keyOptional && !form.api_key?.trim())}>
          <ShieldCheck size={16} /> {busy ? "Sealing..." : "Save sealed"}
        </Button>
      </div>
      <Notice msg={msg} />
    </Card>
  );
}
