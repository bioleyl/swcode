// npm run dev: the Go daemon (built again and restarted when a Go file changes) and the web app (hot reload), side
// by side, stopped together.
import { spawn } from 'node:child_process';
import { once } from 'node:events';
import { watch } from 'node:fs';
import { join } from 'node:path';

// In development people open the Vite server, so the link the daemon prints points there.
const VITE_URL = 'http://127.0.0.1:5173';
const DAEMON_BINARY = join('dist', 'dev', 'swcode');
const COMMAND_LINE = './cmd/swcode';
// The daemon's sources: its packages, and the root package's (what the program carries).
const GO_FOLDERS = [
  { folder: 'cmd', recursive: true },
  { folder: 'daemon', recursive: true },
  { folder: '.', recursive: false },
];
const GO_FILE = '.go';
// A save often writes several files: one rebuild for them all.
const SETTLE_MS = 300;

const relay = (name, stream) => (chunk) =>
  stream.write(
    chunk
      .toString()
      .split('\n')
      .filter((line) => line.length > 0)
      .map((line) => `[${name}] ${line}\n`)
      .join('')
  );

// Each child leads a process group of its own, stopped whole: npm hands no signal on to the Vite it runs.
const started = (name, program, args, environment = {}) => {
  const child = spawn(program, args, {
    detached: true,
    env: { ...process.env, ...environment },
    stdio: ['ignore', 'pipe', 'pipe'],
  });
  child.stdout.on('data', relay(name, process.stdout));
  child.stderr.on('data', relay(name, process.stderr));
  return child;
};

const ended = async (child) => {
  if (child === undefined || child.exitCode !== null || child.signalCode !== null) {
    return;
  }
  const exit = once(child, 'exit');
  process.kill(-child.pid, 'SIGTERM');
  await exit;
};

// The daemon running, the restart waiting for the files to settle, and the restarts in turn: never two at once.
const state = { daemon: undefined, pending: undefined, restarts: Promise.resolve(), stopping: false };

/** Builds the daemon and starts it; a build that fails says why and waits for the next change. */
async function startDaemon() {
  const build = started('build', 'go', ['build', '-o', DAEMON_BINARY, COMMAND_LINE]);
  const [code] = await once(build, 'exit');
  if (code !== 0 || state.stopping) {
    return;
  }
  state.daemon = started('server', DAEMON_BINARY, ['serve'], {
    // biome-ignore lint/style/useNamingConvention: environment variable names
    SWCODE_PUBLIC_URL: process.env.SWCODE_PUBLIC_URL ?? VITE_URL,
  });
}

const restartDaemon = () => {
  clearTimeout(state.pending);
  state.pending = setTimeout(() => {
    state.restarts = state.restarts.then(async () => {
      await ended(state.daemon);
      await startDaemon();
    });
  }, SETTLE_MS);
};

const watchers = GO_FOLDERS.map(({ folder, recursive }) =>
  watch(folder, { recursive }, (_event, file) => {
    if (file?.endsWith(GO_FILE)) {
      restartDaemon();
    }
  })
);
const web = started('web', 'npm', ['run', '--silent', 'dev:web']);
state.restarts = startDaemon();

const stopAll = async () => {
  state.stopping = true;
  clearTimeout(state.pending);
  for (const watcher of watchers) {
    watcher.close();
  }
  await state.restarts;
  await Promise.all([ended(state.daemon), ended(web)]);
};

process.once('SIGINT', stopAll);
process.once('SIGTERM', stopAll);
web.once('exit', (code) => {
  process.exitCode = code ?? 0;
  void stopAll();
});
