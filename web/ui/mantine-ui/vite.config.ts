import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// https://vitejs.dev/config/
export default defineConfig({
  plugins: [react()],
  server: {
    proxy: {
      "/api": {
        target: "https://demo.promlabs.com",
        changeOrigin: true,
        secure: false,
      },
      "/-/": {
        target: "https://demo.promlabs.com",
        changeOrigin: true,
        secure: false,
      },
    },
  },
});
