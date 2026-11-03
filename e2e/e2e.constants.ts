import { join } from 'node:path';

export const ROOT = join(import.meta.dirname, '..');
/** The daemon the tests run, built by the global setup. */
export const DAEMON_BINARY = join(ROOT, 'dist', 'e2e', 'swcode');
/** The built web app (npm run build first). */
export const WEB_DIR = join(ROOT, 'dist', 'web');
