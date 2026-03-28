import { realpathSync } from "fs";
import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

const root = realpathSync(".");

export default defineConfig({
  root,
  plugins: [react()],
  build: {
    outDir: "dist",
    emptyOutDir: true,
  },
});
