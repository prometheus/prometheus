import { defineConfig } from "vitest/config";
import react from "@vitejs/plugin-react";

export default defineConfig({
  plugins: [react()],
  define: {
    GLOBAL_CONSOLES_LINK: '""',
    GLOBAL_AGENT_MODE: '"false"',
    GLOBAL_READY: '"true"',
    GLOBAL_LOOKBACKDELTA: '""',
  },
  test: {
    globals: true,
    environment: "jsdom",
    setupFiles: "./src/setupTests.ts",
  },
});
