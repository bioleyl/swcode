import { defineConfig } from 'vite';

const DAEMON = process.env.SWCODE_DAEMON ?? 'http://127.0.0.1:4500';

// npm run dev:web: the app with hot reload; its API calls and its socket go to the daemon from npm run dev:server.
export default defineConfig({
  build: { emptyOutDir: true, outDir: '../dist/web' },
  oxc: { jsx: { importSource: '@jadis/core', runtime: 'automatic' } },
  root: import.meta.dirname,
  server: { port: 5173, proxy: { '/api': { changeOrigin: true, target: DAEMON, ws: true } } },
});
