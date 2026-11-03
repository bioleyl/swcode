import { execFileSync } from 'node:child_process';

import { DAEMON_BINARY, ROOT } from './e2e.constants.js';

/** Vitest's global setup: the Go daemon built once for the whole run, as the person's would be. */
export default function setup(): void {
  execFileSync('go', ['build', '-o', DAEMON_BINARY, './cmd/swcode'], { cwd: ROOT, stdio: 'inherit' });
}
