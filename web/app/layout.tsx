import type { Metadata } from "next";
import "./globals.css";
import { Sidebar } from "@/components/sidebar";
import { HealthPill } from "@/components/health-pill";

export const metadata: Metadata = {
  title: "Korugan",
  description: "AI-native edge security operating layer",
};

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="en">
      <body className="min-h-[100dvh] bg-bg text-slate-200 antialiased">
        <div className="flex min-h-[100dvh]">
          <Sidebar />
          <div className="flex min-w-0 flex-1 flex-col">
            <header className="flex h-16 items-center justify-between border-b border-line px-8">
              <div className="text-sm text-slate-500">
                Watch the edge. Explain the noise. Fix with consent.
              </div>
              <HealthPill />
            </header>
            <main className="mx-auto w-full max-w-[1400px] flex-1 px-8 py-8">{children}</main>
          </div>
        </div>
      </body>
    </html>
  );
}
