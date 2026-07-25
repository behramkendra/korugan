"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import {
  SquaresFour,
  ListBullets,
  Warning,
  Lightbulb,
  ChatCircleText,
  ShieldCheck,
} from "@phosphor-icons/react";

const nav = [
  { href: "/", label: "Dashboard", icon: SquaresFour },
  { href: "/events", label: "Events", icon: ListBullets },
  { href: "/findings", label: "Findings", icon: Warning },
  { href: "/recommendations", label: "Recommendations", icon: Lightbulb },
  { href: "/chat", label: "Chat", icon: ChatCircleText },
];

export function Sidebar() {
  const path = usePathname();
  return (
    <aside className="flex w-60 shrink-0 flex-col border-r border-line bg-surface">
      <div className="flex items-center gap-2 px-5 py-5">
        <ShieldCheck size={26} weight="duotone" className="text-accent" />
        <div>
          <div className="font-semibold tracking-tight text-slate-100">Korugan</div>
          <div className="text-[11px] uppercase tracking-wider text-slate-500">Edge Security</div>
        </div>
      </div>
      <nav className="flex flex-1 flex-col gap-1 px-3 py-2">
        {nav.map(({ href, label, icon: Icon }) => {
          const active = href === "/" ? path === "/" : path.startsWith(href);
          return (
            <Link
              key={href}
              href={href}
              className={`flex items-center gap-3 rounded-lg px-3 py-2 text-sm transition ${
                active
                  ? "bg-surface-2 text-accent"
                  : "text-slate-400 hover:bg-surface-2 hover:text-slate-200"
              }`}
            >
              <Icon size={18} weight={active ? "fill" : "regular"} />
              {label}
            </Link>
          );
        })}
      </nav>
      <div className="border-t border-line px-5 py-4 text-[11px] text-slate-600">
        Self-hosted. BYOK. Read-only by default.
      </div>
    </aside>
  );
}
