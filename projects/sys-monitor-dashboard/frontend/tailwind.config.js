/** @type {import('tailwindcss').Config} */
export default {
  content: [
    "./index.html",
    "./src/**/*.{js,ts,jsx,tsx}",
  ],
  theme: {
    extend: {
      // Minimalist, high-contrast palette with subtle pastel accents for log levels.
      colors: {
        // Neutral base — clean white background, near-black text for max contrast.
        ink: {
          DEFAULT: "#0f172a", // slate-900 — primary text
          soft: "#475569",    // slate-600 — secondary text
          faint: "#94a3b8",   // slate-400 — muted text
        },
        canvas: {
          DEFAULT: "#ffffff", // pure white surface
          subtle: "#f8fafc",  // slate-50 — alt rows / panels
          line: "#e2e8f0",    // slate-200 — borders / dividers
        },

        // Pastel log-level accents — soft but distinguishable.
        "pastel-red": {
          DEFAULT: "#fecaca", // red-200 — ERROR
          soft: "#fee2e2",    // red-100 — ERROR bg tint
          text: "#b91c1c",    // red-700 — ERROR fg (AA contrast on white)
        },
        "pastel-yellow": {
          DEFAULT: "#fef08a", // yellow-200 — WARN
          soft: "#fef9c3",    // yellow-100 — WARN bg tint
          text: "#a16207",    // yellow-700 — WARN fg (AA contrast on white)
        },
        "pastel-green": {
          DEFAULT: "#bbf7d0", // green-200 — INFO / success
          soft: "#dcfce7",    // green-100 — INFO bg tint
          text: "#15803d",    // green-700 — INFO fg (AA contrast on white)
        },
        "pastel-blue": {
          DEFAULT: "#bfdbfe", // blue-200 — DEBUG / neutral info
          soft: "#dbeafe",    // blue-100 — DEBUG bg tint
          text: "#1d4ed8",    // blue-700 — DEBUG fg (AA contrast on white)
        },
      },
      fontFamily: {
        // Monospace for log lines; system sans for everything else.
        sans: [
          "Inter",
          "ui-sans-serif",
          "system-ui",
          "-apple-system",
          "Segoe UI",
          "Roboto",
          "Helvetica Neue",
          "Arial",
          "sans-serif",
        ],
        mono: [
          "JetBrains Mono",
          "ui-monospace",
          "SFMono-Regular",
          "Menlo",
          "Consolas",
          "monospace",
        ],
      },
      borderRadius: {
        // Soft but restrained corners for a minimalist feel.
        DEFAULT: "0.375rem",
        lg: "0.5rem",
        xl: "0.75rem",
      },
      boxShadow: {
        // Minimal elevation — barely-there borders over heavy shadows.
        subtle: "0 1px 2px 0 rgba(15, 23, 42, 0.04)",
        panel: "0 1px 3px 0 rgba(15, 23, 42, 0.06), 0 1px 2px -1px rgba(15, 23, 42, 0.04)",
      },
      fontSize: {
        // Tight, scannable type scale.
        xxs: ["0.6875rem", { lineHeight: "1rem", letterSpacing: "0.02em" }],
      },
      letterSpacing: {
        tightish: "-0.01em",
      },
    },
  },
  plugins: [],
};
