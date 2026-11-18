import { defineConfig } from "vite";
export default defineConfig({
  base: "/",
  build: {
    outDir: "../internal/dashboard/assets",
    emptyOutDir: true,
    sourcemap: false,
  },
});
