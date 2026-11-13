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
export default defineConfig({
  build: { emptyOutDir: true, outDir: OUT_DIR },
  oxc: { jsx: { importSource: '@jadis/core', runtime: 'automatic' } },
  plugins: [compressedCopies()],
  root: import.meta.dirname,
  server: { port: 5173, proxy: { '/api': { changeOrigin: true, target: DAEMON, ws: true } } },
});
