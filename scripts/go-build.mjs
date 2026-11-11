// What the Go builds share: the version a program carries, this machine's platform as Go names it, and go build
// itself, the web app inside the program (npm run build makes it first).
import { execFileSync } from 'node:child_process';
import { readFileSync } from 'node:fs';
import { join } from 'node:path';

const GO_SYSTEMS = { darwin: 'darwin', linux: 'linux', win32: 'windows' };
const GO_ARCHITECTURES = { arm64: 'arm64', x64: 'amd64' };
const WINDOWS = 'windows';
const VERSION_VARIABLE = 'swcode/daemon/version.Build';
// The web app inside the program (webapp_embed.go).
const EMBED_WEB = 'embedweb';
// No symbol tables.
const SMALL = ['-s', '-w'];

/** This machine's platform: linux-amd64, darwin-arm64… */
export const HERE = `${GO_SYSTEMS[process.platform]}-${GO_ARCHITECTURES[process.arch]}`;

export const packageVersion = () => JSON.parse(readFileSync('package.json', 'utf8')).version;

/** The version and the commit, as swcode --version says them. */
const buildId = () => {
  const commit = execFileSync('git', ['rev-parse', '--short', 'HEAD']).toString().trim();
  return `${packageVersion()}+${commit}`;
};

/** Where the command line for a platform goes: the release publishes each one. */
export const binaryOf = (platform) =>
  join('dist', `swcode-${platform}${platform.startsWith(WINDOWS) ? '.exe' : ''}`);

/** Builds a Go package for a platform, the web app and its version inside; extra linker flags come first. */
export function goBuild({ linkerFlags = [], out, target, platform }) {
  const [system, architecture] = platform.split('-');
  const flags = [...linkerFlags, ...SMALL, '-X', `${VERSION_VARIABLE}=${buildId()}`].join(' ');
  execFileSync('go', ['build', '-trimpath', '-tags', EMBED_WEB, '-ldflags', flags, '-o', out, target], {
    // biome-ignore lint/style/useNamingConvention: environment variable names
    env: { ...process.env, CGO_ENABLED: '0', GOARCH: architecture, GOOS: system },
    stdio: 'inherit',
  });
}
