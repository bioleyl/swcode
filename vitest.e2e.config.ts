import { defineConfig } from 'vitest/config';

// npm run test:e2e: the built app in a real browser, against the Go daemon and a fake llm-server.
export default defineConfig({
  test: {
    globalSetup: ['e2e/build-daemon.ts'],
    hookTimeout: 60_000,
    include: ['e2e/**/*.e2e.test.ts'],
    testTimeout: 60_000,
  },
});
