import type { Config } from "tailwindcss";

// Korugan dark-locked palette: slate base, cyan accent, amber warning.
// One accent, one radius scale (rounded-lg = 8px), consistent everywhere.
const config: Config = {
  content: ["./app/**/*.{ts,tsx}", "./components/**/*.{ts,tsx}"],
  theme: {
    extend: {
      colors: {
        bg: "#0a1020",
        surface: "#0d1730",
        "surface-2": "#111c38",
        line: "#1e2f52",
        accent: "#22d3ee",
        "accent-dim": "#0e7490",
      },
      fontFamily: {
        sans: ["ui-sans-serif", "system-ui", "Segoe UI", "Roboto", "sans-serif"],
        mono: ["ui-monospace", "SFMono-Regular", "Menlo", "monospace"],
      },
    },
  },
  plugins: [],
};

export default config;
