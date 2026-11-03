// npm run install:binary: this machine's binary, just built, installs itself (see swcode install).
import { execFileSync } from 'node:child_process';

import { binaryOf, HERE } from './go-build.mjs';

execFileSync(binaryOf(HERE), ['install'], { stdio: 'inherit' });
