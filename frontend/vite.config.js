import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// The engine's own client modules (app/*.js: the API client, the WebSocket feed, the demo engine and
// its rules) are plain JavaScript with no framework in them. They stay where they are, are tested by
// node --test, and the React app in src/ imports them.
export default defineConfig({
  plugins: [react()],
  server: { port: 5173, strictPort: true },
  build: { outDir: "dist", sourcemap: false, chunkSizeWarningLimit: 700 },
});
