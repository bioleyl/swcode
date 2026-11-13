import { defineConfig } from 'vitest/config';

// npm run test:e2e: the built app in a real browser, against the Go daemon and a fake llm-server.
export default defineConfig({
  test: {
    // A poll waits for the page, the daemon and a browser that share the machine with the other test files: one
    // second, vitest's own limit, is not enough as soon as it is busy. A poll that succeeds returns at once, so the
    // suite is no slower for it.
    expect: { poll: { timeout: 15_000 } },
    globalSetup: ['e2e/build-daemon.ts'],
    hookTimeout: 60_000,
    include: ['e2e/**/*.e2e.test.ts'],
    testTimeout: 60_000,
  },
});
