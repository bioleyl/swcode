// npm run build:desktop: swcode for Windows as one desktop app, one program: its window, a small Go program on
// Edge's WebView2 (desktop/), with the server and the web app inside it. go-winres writes its icon, its names and
// its manifest into it: no Windows needed.
import { execFileSync } from 'node:child_process';
import { rmSync } from 'node:fs';
import { join, resolve } from 'node:path';

import { goBuild, packageVersion } from './go-build.mjs';

const DESKTOP = 'desktop';
const PLATFORM = 'windows-amd64';
const APP_EXE = 'swcode-desktop.exe';
const OUT = join('dist', APP_EXE);
const ICON = 'web/public/swcode.ico';
// go build links the .syso files of the package's folder into the program.
const RESOURCES = 'rsrc';
const RESOURCES_FILE = join(DESKTOP, `${RESOURCES}_windows_amd64.syso`);
// A window, not a console.
const WINDOW = ['-H', 'windowsgui'];

// Windows wants four numbers.
const windowsVersion = () => `${packageVersion()}.0`;

execFileSync(
  'go',
  [
    '-C',
    DESKTOP,
    'tool',
    'go-winres',
    'simply',
    '--arch',
    'amd64',
    '--out',
    RESOURCES,
    '--icon',
    resolve(ICON),
    '--manifest',
    'gui',
    '--product-name',
    'swcode',
    '--file-description',
    'swcode',
    '--copyright',
    'SyWare',
    '--original-filename',
    APP_EXE,
    '--product-version',
    windowsVersion(),
    '--file-version',
    windowsVersion(),
  ],
  { stdio: 'inherit' }
);
try {
  goBuild({ linkerFlags: WINDOW, out: OUT, platform: PLATFORM, target: `./${DESKTOP}` });
} finally {
  rmSync(RESOURCES_FILE, { force: true });
}
console.log(`Built ${OUT}: the window, with its server inside.`);
