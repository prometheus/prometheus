import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// https://vitejs.dev/config/
export default defineConfig({
  base: '',
  plugins: [react()],
  server: {
    proxy: {
      "/api": {
        target: "http://localhost:9090",
      },
      "/-/": {
        target: "http://localhost:9090",
      },
      // "/api": {
      //   target: "https://prometheus.demo.do.prometheus.io/",
      //   changeOrigin: true,
      // },
      // "/-/": {
      //   target: "https://prometheus.demo.do.prometheus.io/",
      //   changeOrigin: true,
      // },
    },
  },
});
