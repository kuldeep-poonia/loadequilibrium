import type { Config } from "tailwindcss";

const config: Config = {
  content: ["./src/**/*.{ts,tsx}"],
  theme: {
    extend: {
      fontFamily: {
        mono: ["'JetBrains Mono'", "'Fira Code'", "monospace"],
      },
      colors: {
        surface: {
          0: "#0a0b0f",
          1: "#10121a",
          2: "#161927",
          3: "#1e2236",
          4: "#252b42",
        },
        brand: {
          DEFAULT: "#4f8ef7",
          dim: "#2d5ab4",
          muted: "#1e3a7a",
        },
        success: "#22c55e",
        warning: "#f59e0b",
        danger: "#ef4444",
        critical: "#dc2626",
        muted: "#4b5563",
        subtle: "#6b7280",
        text: {
          primary: "#e2e8f0",
          secondary: "#94a3b8",
          tertiary: "#64748b",
        },
      },
      animation: {
        pulse2: "pulse 2s cubic-bezier(0.4,0,0.6,1) infinite",
        fadein: "fadein 0.2s ease",
      },
      keyframes: {
        fadein: {
          "0%": { opacity: "0", transform: "translateY(4px)" },
          "100%": { opacity: "1", transform: "translateY(0)" },
        },
      },
    },
  },
  plugins: [],
};

export default config;
