import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// Wails serves the built assets, so output goes to dist and paths stay relative.
export default defineConfig({
  plugins: [react()],
  base: "./",
  build: {
    outDir: "dist",
  },
});
