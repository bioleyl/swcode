import { execFileSync } from 'node:child_process';

import { DAEMON_BINARY, E2E_VERSION, ROOT } from './e2e.constants.js';

const VERSION_FLAG = `-X swcode/daemon/version.Build=${E2E_VERSION}`;

/** Vitest's global setup: the Go daemon built once for the whole run, as the person's would be, a release's. */
export default function setup(): void {
  execFileSync('go', ['build', '-ldflags', VERSION_FLAG, '-o', DAEMON_BINARY, './cmd/swcode'], {
    cwd: ROOT,
    stdio: 'inherit',
  });
}
