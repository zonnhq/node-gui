/** Drapery Drama: derived from the four anchors (Pewter, Black, Champagne, Dark
 * Blue) in documentation/zonn/blueprint.md Section 8.1. Named keys are kept
 * from the original Colony palette so class names across the app did not need
 * to change, only what each one renders. */
export default {
  content: ["./index.html", "./src/**/*.{ts,tsx}"],
  theme: {
    extend: {
      colors: {
        colony: {
          navy: "#1A1720",
          core: "#544943",
          deep: "#2F2A2D",
          ice: "#F0D2A3",
          cloud: "#C6C5BF",
          sand: "#C9C6BD",
          charcoal: "#0A0A0C",
          slate: "#48464B",
          mist: "#878782",
          paleblue: "#D0C8B8",
          softblue: "#9F9E99",
          lightblue: "#BAB9B4",
          nearwhite: "#CAC6BC",
          olive: "#453C3A",
          warmbeige: "#D7CAB4",
          coolgray: "#8B8B86",
          grayblue: "#676568",
          midblue: "#65584E",
          indigo: "#0F0E12",
          vibrant: "#EAD0A7",
          midnight: "#070707",
          black: "#020301",
        },
      },
      fontFamily: {
        sans: ["JetBrains Mono", "Cascadia Code", "Ubuntu Mono", "DejaVu Sans Mono", "ui-monospace", "SF Mono", "Menlo", "Consolas", "monospace"],
        mono: ["JetBrains Mono", "Cascadia Code", "Ubuntu Mono", "DejaVu Sans Mono", "ui-monospace", "SF Mono", "Menlo", "Consolas", "monospace"],
      },
    },
  },
  plugins: [],
};
