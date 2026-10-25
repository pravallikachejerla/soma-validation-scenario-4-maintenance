import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

export default defineConfig({
  plugins: [react()],
  server: {
    port: 5173,
    host: "0.0.0.0",
    proxy: {
      "/api": "http://localhost:8080",
      "/healthz": "http://localhost:8080",
      "/version": "http://localhost:8080",
      "/metrics": "http://localhost:8080",
    },
  },
  preview: {
    port: 5173,
    host: "0.0.0.0",
  },
});
