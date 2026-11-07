import { join } from 'node:path';

export const ROOT = join(import.meta.dirname, '..');
/** The version the daemon the tests run says it is: a release's, so it looks for a newer one. */
export const E2E_RELEASE = '0.1.0';
export const E2E_VERSION = `${E2E_RELEASE}+e2e`;
/** The daemon the tests run, built by the global setup. */
export const DAEMON_BINARY = join(ROOT, 'dist', 'e2e', 'swcode');
/** The built web app (npm run build first). */
export const WEB_DIR = join(ROOT, 'dist', 'web');
