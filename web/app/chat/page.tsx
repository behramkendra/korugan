"use client";

import { useRef, useState } from "react";
import { PaperPlaneRight, ChatCircleText, Sparkle } from "@phosphor-icons/react";
import { api, ApiError } from "@/lib/api";
import { SectionHeader, Card, Button } from "@/components/ui";

type Turn = { role: "user" | "korugan"; text: string; grounded?: number };

const suggestions = [
  "Why did blocked traffic spike in the last 24 hours?",
  "What are my highest-severity findings right now?",
  "Which zones look misconfigured?",
];

export default function ChatPage() {
  const [turns, setTurns] = useState<Turn[]>([]);
  const [input, setInput] = useState("");
  const [busy, setBusy] = useState(false);
  const [notice, setNotice] = useState<string | null>(null);
  const endRef = useRef<HTMLDivElement>(null);

  async function send(message: string) {
    const q = message.trim();
    if (!q || busy) return;
    setNotice(null);
    setTurns((t) => [...t, { role: "user", text: q }]);
    setInput("");
    setBusy(true);
    try {
      const r = await api.chat(q);
      setTurns((t) => [...t, { role: "korugan", text: r.answer, grounded: r.grounded_events }]);
      requestAnimationFrame(() => endRef.current?.scrollIntoView({ behavior: "smooth" }));
    } catch (e) {
      if (e instanceof ApiError && e.status === 503) {
        setNotice(
          "Chat needs an LLM key. Korugan is in zero-key mode. Add a key (OpenRouter, Anthropic, OpenAI, DeepSeek or Ollama) to enable AI answers.",
        );
      } else {
        setNotice(e instanceof ApiError ? e.message : String(e));
      }
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="flex h-[calc(100dvh-8rem)] flex-col">
      <SectionHeader
        title="Ask Korugan"
        subtitle="Grounded in your own events and findings. Answers cite the data they come from."
      />

      <Card className="flex min-h-0 flex-1 flex-col">
        <div className="flex-1 space-y-4 overflow-y-auto p-5">
          {turns.length === 0 ? (
            <div className="flex h-full flex-col items-center justify-center text-center">
              <ChatCircleText size={40} weight="duotone" className="mb-3 text-slate-600" />
              <p className="text-sm text-slate-400">Ask about your traffic, security events, or findings.</p>
              <div className="mt-5 flex flex-wrap justify-center gap-2">
                {suggestions.map((s) => (
                  <button
                    key={s}
                    onClick={() => send(s)}
                    className="rounded-lg border border-line px-3 py-1.5 text-xs text-slate-300 hover:bg-surface-2"
                  >
                    {s}
                  </button>
                ))}
              </div>
            </div>
          ) : (
            turns.map((t, i) => (
              <div key={i} className={t.role === "user" ? "flex justify-end" : "flex justify-start"}>
                <div
                  className={`max-w-[80%] rounded-lg px-4 py-3 text-sm ${
                    t.role === "user"
                      ? "bg-accent/15 text-slate-100"
                      : "border border-line bg-surface-2 text-slate-200"
                  }`}
                >
                  {t.role === "korugan" ? (
                    <div className="mb-1 flex items-center gap-1.5 text-xs text-accent">
                      <Sparkle size={12} weight="fill" /> Korugan
                    </div>
                  ) : null}
                  <div className="whitespace-pre-wrap leading-relaxed">{t.text}</div>
                  {t.grounded !== undefined ? (
                    <div className="mt-2 text-[11px] text-slate-500">
                      grounded in {t.grounded} event{t.grounded === 1 ? "" : "s"}
                    </div>
                  ) : null}
                </div>
              </div>
            ))
          )}
          {busy ? (
            <div className="flex justify-start">
              <div className="rounded-lg border border-line bg-surface-2 px-4 py-3 text-sm text-slate-500">
                <span className="inline-flex gap-1">
                  <span className="h-1.5 w-1.5 animate-bounce rounded-full bg-slate-500 [animation-delay:0ms]" />
                  <span className="h-1.5 w-1.5 animate-bounce rounded-full bg-slate-500 [animation-delay:120ms]" />
                  <span className="h-1.5 w-1.5 animate-bounce rounded-full bg-slate-500 [animation-delay:240ms]" />
                </span>
              </div>
            </div>
          ) : null}
          <div ref={endRef} />
        </div>

        {notice ? (
          <div className="mx-5 mb-3 rounded-lg border border-amber-500/30 bg-amber-500/10 p-3 text-xs text-amber-200">
            {notice}
          </div>
        ) : null}

        <form
          onSubmit={(e) => {
            e.preventDefault();
            send(input);
          }}
          className="flex items-center gap-2 border-t border-line p-3"
        >
          <input
            value={input}
            onChange={(e) => setInput(e.target.value)}
            placeholder="Ask about your edge..."
            className="flex-1 rounded-lg border border-line bg-bg px-4 py-2.5 text-sm text-slate-200 placeholder:text-slate-600 focus:border-accent/50 focus:outline-none"
          />
          <Button type="submit" disabled={busy || !input.trim()}>
            <PaperPlaneRight size={16} /> Send
          </Button>
        </form>
      </Card>
    </div>
  );
}
