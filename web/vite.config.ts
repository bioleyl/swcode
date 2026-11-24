import { readdirSync, readFileSync, writeFileSync } from 'node:fs';
import { join, resolve } from 'node:path';
import { brotliCompressSync, gzipSync } from 'node:zlib';

import { defineConfig } from 'vite';

import type { Plugin } from 'vite';

const DAEMON = process.env.SWCODE_DAEMON ?? 'http://127.0.0.1:4500';
const OUT_DIR = resolve(import.meta.dirname, '../dist/web');
// The files of the build that are text: fonts and pictures are compressed already.
const TEXT_FILE = /\.(?:css|html|ico|js|json|svg|webmanifest)$/;

/**
 * Leaves a brotli and a gzip copy beside each text file of the build. The daemon sends the copy a browser reads
 * (daemon/app/webapp.go) without compressing anything itself: the app is a quarter of its size on the way to a
 * phone.
 */
const compressedCopies = (): Plugin => ({
  apply: 'build',
  closeBundle() {
    for (const name of readdirSync(OUT_DIR, { recursive: true })
      .map(String)
      .filter((each) => TEXT_FILE.test(each))) {
      const file = join(OUT_DIR, name);
      const content = readFileSync(file);
      writeFileSync(`${file}.br`, brotliCompressSync(content));
      writeFileSync(`${file}.gz`, gzipSync(content));
    }
  },
  name: 'swcode-compressed-copies',
});

// npm run dev:web: the app with hot reload; its API calls and its socket go to the daemon from npm run dev:server.
// The two entries: the app's page, and the bundle a host ships (web/vscode.html, the VS Code extension's webview).
const ENTRIES = {
  app: resolve(import.meta.dirname, 'index.html'),
  vscode: resolve(import.meta.dirname, 'vscode.html'),
};

export default defineConfig({
  build: {
    chunkSizeWarningLimit: 800,
    emptyOutDir: true,
    outDir: OUT_DIR,
    rollupOptions: { input: ENTRIES },
  },
  oxc: { jsx: { importSource: '@jadis/core', runtime: 'automatic' } },
  plugins: [compressedCopies()],
  root: import.meta.dirname,
  server: { port: 5173, proxy: { '/api': { changeOrigin: true, target: DAEMON, ws: true } } },
});
