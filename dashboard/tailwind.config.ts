import type { Config } from "tailwindcss";

const config: Config = {
  content: [
    "./src/pages/**/*.{js,ts,jsx,tsx,mdx}",
    "./src/components/**/*.{js,ts,jsx,tsx,mdx}",
    "./src/app/**/*.{js,ts,jsx,tsx,mdx}",
  ],
  theme: {
    extend: {
      colors: {
        background: "var(--background)",
        foreground: "var(--foreground)",
        "hud-cyan": "#00f2ff",
        "hud-red": "#ff3366",
      },
      fontFamily: {
        hud: ["var(--font-hud)", "sans-serif"],
        data: ["var(--font-data)", "monospace"],
        lbl: ["var(--font-lbl)", "sans-serif"],
      },
      boxShadow: {
        "inner-cyan": "inset 0 0 30px rgba(0, 242, 255, 0.05)",
      },
      dropShadow: {
        "cyan": "0 0 20px rgba(0, 242, 255, 0.4)",
        "cyan-lo": "0 0 10px rgba(0, 242, 255, 0.2)",
      },
      animation: {
        "scan": "scan 12s linear infinite",
      },
      keyframes: {
        scan: {
          "from": { top: "-200px" },
          "to": { top: "110%" },
        }
      }
    },
  },
  plugins: [],
};
export default config;
