// npm run install:desktop, from WSL: puts the desktop app built by npm run build:desktop in Windows' programs
// folder, %LOCALAPPDATA%\Programs\swcode, in place of what was there: an older app, with the swcode.exe server it
// had beside it, or the swcode.exe daemon of before, stopped first. The app makes its Start menu and desktop
// shortcuts itself when it starts.
import { execFileSync } from 'node:child_process';
import { copyFileSync, existsSync, mkdirSync, readdirSync, rmSync } from 'node:fs';
import { join } from 'node:path';

const APP_EXE = 'swcode-desktop.exe';
const BUILD = join('dist', APP_EXE);
// The server older apps had beside them; the command line's daemon, before the app, which may still run.
const SERVER_EXE = 'swcode.exe';
const FOLDER = ['Programs', 'swcode'];

const powershell = (command) =>
  execFileSync('powershell.exe', ['-NoProfile', '-Command', command]).toString().replace(/\r/g, '').trim();
const unixPath = (windows) => execFileSync('wslpath', ['-u', windows]).toString().trim();
const windowsPath = (unix) => execFileSync('wslpath', ['-w', unix]).toString().trim();
const runs = (exe) => powershell(`@(Get-CimInstance Win32_Process -Filter "name='${exe}'").Count`) !== '0';

if (!existsSync(BUILD)) {
  throw new Error(`${BUILD} is missing: npm run build:desktop makes it.`);
}
if (runs(APP_EXE)) {
  console.error('swcode is open: close it, then install again.');
  process.exit(1);
}
const folder = join(unixPath(powershell("[Environment]::GetFolderPath('LocalApplicationData')")), ...FOLDER);
const server = join(folder, SERVER_EXE);
if (existsSync(server) && runs(SERVER_EXE)) {
  execFileSync(server, ['stop'], { stdio: 'inherit' });
}
for (const entry of existsSync(folder) ? readdirSync(folder) : []) {
  rmSync(join(folder, entry), { force: true, recursive: true });
}
mkdirSync(folder, { recursive: true });
copyFileSync(BUILD, join(folder, APP_EXE));
// The folder leaves the PATH the command line's install put it on: what it holds is the app now.
const folderOnWindows = windowsPath(folder);
powershell(
  [
    "$path = [Environment]::GetEnvironmentVariable('Path', 'User')",
    `$kept = ($path -split ';') | Where-Object { $_ -and $_ -ne '${folderOnWindows}' }`,
    "[Environment]::SetEnvironmentVariable('Path', ($kept -join ';'), 'User')",
  ].join('; ')
);
console.log(`Installed swcode in ${folderOnWindows}: open ${APP_EXE} once, it makes its shortcuts.`);
