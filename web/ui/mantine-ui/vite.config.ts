import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import license from "rollup-plugin-license";
import path from "path";

// https://vitejs.dev/config/
export default defineConfig({
  base: '',
  plugins: [react()],
  build: {
    rollupOptions: {
      plugins: [
        // Collect the licenses of all third-party packages that end up in the
        // bundle and write them to a single file that is embedded and shipped
        // with Prometheus, satisfying their attribution requirements.
        license({
          thirdParty: {
            includePrivate: false,
            output: {
              file: path.resolve(
                __dirname,
                "dist",
                "assets",
                "third-party-licenses.txt"
              ),
            },
          },
        }),
      ],
    },
  },
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
