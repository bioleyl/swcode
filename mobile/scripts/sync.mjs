// Copies the web app the root build made into www, then has Capacitor put it in the Android project. The release's
// APK is built from these assets, so it shows the same pages as the daemon serves.
import { cpSync, mkdirSync, rmSync } from 'node:fs';
import { execFileSync } from 'node:child_process';
import { resolve } from 'node:path';

const source = resolve(import.meta.dirname, '../../dist/web');
const target = resolve(import.meta.dirname, '../www');

rmSync(target, { recursive: true, force: true });
mkdirSync(target, { recursive: true });
cpSync(source, target, { recursive: true });
execFileSync('npx', ['cap', 'sync', 'android'], { stdio: 'inherit' });
