/** biome-ignore-all lint/style/useNamingConvention: environment variable names */
import { spawn } from 'node:child_process';
import { once } from 'node:events';
import { copyFileSync, mkdirSync, mkdtempSync, readdirSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { createServer } from 'node:net';
import { tmpdir } from 'node:os';
import { delimiter, dirname, join } from 'node:path';
import { setTimeout as sleep } from 'node:timers/promises';

import { chromium } from 'playwright-core';

import { ENDPOINTS, TOKEN_PARAM } from '../shared/protocol/endpoints.js';
import { SessionEventSchema } from '../shared/protocol/session.schema.js';
import { DAEMON_BINARY, E2E_RELEASE, WEB_DIR } from './e2e.constants.js';
import { startFakeLlama } from './fake-llama.js';
import { startFakeReleases } from './fake-releases.js';

import type { ChildProcess } from 'node:child_process';
import type { SessionEvent } from '../shared/protocol/session.schema.js';
import type { E2eOptions, E2eRig } from './types/e2e.types.js';

const TOKEN = 'e2e-token-0123456789abcdef';
const MODEL = 'gpt-oss-120b';
const CONTEXT_TOKENS = 65_536;
const SLOTS = 2;
const HOST = '127.0.0.1';
const SESSIONS_FOLDER = 'sessions';
const EVENTS_SUFFIX = '.jsonl';
const START_TIMEOUT_MS = 20_000;
const POLL_MS = 50;
// The latest release when the test sets none: the one the daemon is, which then has nothing newer to offer.
const SAME_RELEASE = { notes: '', version: E2E_RELEASE };
// Where a daemon that may install a newer version runs from, in its home: the person's command line is there.
const PROGRAM_PATH = '.local/bin/swcode';

// The folder of the daemon's home whose programs come first: the stand-ins for those of the machine.
const FAKE_BIN = 'bin';
const EXECUTABLE = 0o755;
/**
 * Tailscale's command line as the daemon asks it: a tailnet with certificates, which serves a port once asked to.
 * The machine's own Tailscale stays out of the tests, and none of them changes what it serves.
 */
const FAKE_TAILSCALE = `#!/bin/sh
served="$(dirname "$0")/tailnet-port"
case "$*" in
  "status --json") echo '{"BackendState":"Running","CertDomains":["laptop.tail1234.ts.net"]}' ;;
  "serve status --json")
    if [ -f "$served" ]; then
      port="$(cat "$served")"
      printf '{"Web":{"laptop.tail1234.ts.net:%s":{"Handlers":{"/":{"Proxy":"http://127.0.0.1:%s"}}}}}\\n' "$port" "$port"
    else
      echo '{}'
    fi ;;
  "serve --bg --https="*) echo "\${3#--https=}" > "$served" ;;
esac
`;

/** Puts the stand-in programs in the home: the folder to name first on the daemon's PATH. */
const fakePrograms = (home: string): string => {
  const folder = join(home, FAKE_BIN);
  mkdirSync(folder, { recursive: true });
  writeFileSync(join(folder, 'tailscale'), FAKE_TAILSCALE, { mode: EXECUTABLE });
  return folder;
};

const writeAll = (folder: string, files: Readonly<Record<string, string>>): void => {
  for (const [path, content] of Object.entries(files)) {
    mkdirSync(dirname(join(folder, path)), { recursive: true });
    writeFileSync(join(folder, path), content);
  }
};

/** A port nothing listens on, for the daemon to take. */
async function freePort(): Promise<number> {
  const server = createServer().listen(0, HOST);
  await once(server, 'listening');
  const address = server.address();
  server.close();
  return typeof address === 'object' && address !== null ? address.port : 0;
}

const isHealthy = (address: string): Promise<boolean> =>
  fetch(`${address}${ENDPOINTS.health}`).then(
    (response) => response.ok,
    () => false
  );

/** Waits until the daemon answers, and fails with what it said when it ends first or takes too long. */
async function untilReady(daemon: ChildProcess, address: string, said: () => string): Promise<void> {
  const deadline = Date.now() + START_TIMEOUT_MS;
  while (!(await isHealthy(address))) {
    if (daemon.exitCode !== null || Date.now() > deadline) {
      throw new Error(`The daemon did not start:\n${said()}`);
    }
    await sleep(POLL_MS);
  }
}

async function stopDaemon(daemon: ChildProcess): Promise<void> {
  if (daemon.exitCode !== null) {
    return;
  }
  const ended = once(daemon, 'exit');
  daemon.kill('SIGTERM');
  await ended;
}

/** The daemon's program: the one built for the run, or a copy of its own in its home when an update may replace it. */
function programIn(home: string, replaceable: boolean): string {
  if (!replaceable) {
    return DAEMON_BINARY;
  }
  const program = join(home, PROGRAM_PATH);
  mkdirSync(dirname(program), { recursive: true });
  copyFileSync(DAEMON_BINARY, program);
  return program;
}

/** The events of the daemon's sessions, as it keeps them in its data folder. */
const eventsIn = (dataDir: string): SessionEvent[] => {
  const folder = join(dataDir, SESSIONS_FOLDER);
  return readdirSync(folder)
    .filter((name) => name.endsWith(EVENTS_SUFFIX))
    .flatMap((name) => readFileSync(join(folder, name), 'utf8').split('\n'))
    .filter((line) => line !== '')
    .map((line) => SessionEventSchema.parse(JSON.parse(line)));
};

/**
 * The Go daemon on temporary folders serving the built app (npm run build first), a fake llm-server playing the
 * replies, and a headless Chromium: SWCODE_CHROMIUM names its executable, else Playwright's own is used.
 */
export async function startRig(options: E2eOptions): Promise<E2eRig> {
  const workspace = mkdtempSync(join(tmpdir(), 'swcode-e2e-workspace-'));
  const dataDir = mkdtempSync(join(tmpdir(), 'swcode-e2e-data-'));
  // A home of its own, where the person's skills are (~/.agents/skills): theirs stay out.
  const home = mkdtempSync(join(tmpdir(), 'swcode-e2e-home-'));
  writeAll(workspace, options.files ?? {});
  writeAll(home, options.homeFiles ?? {});
  writeAll(dataDir, options.dataFiles ?? {});
  const model = options.model ?? MODEL;
  const llama = await startFakeLlama({
    contextTokens: CONTEXT_TOKENS,
    models: [model],
    replies: options.replies,
    ...(options.scripts ? { scripts: options.scripts } : {}),
    slots: options.slots ?? SLOTS,
    vision: options.vision ?? false,
  });
  const releases = await startFakeReleases(options.release ?? SAME_RELEASE);
  const program = programIn(home, options.release !== undefined);
  const port = await freePort();
  const chromiumPath = process.env.SWCODE_CHROMIUM;
  const daemon = spawn(program, ['serve'], {
    env: {
      HOME: home,
      LLAMA_MODEL: model,
      LLAMA_URL: llama.url,
      PATH: `${fakePrograms(home)}${delimiter}${process.env.PATH ?? ''}`,
      SWCODE_DATA_DIR: dataDir,
      SWCODE_ENV: 'test',
      SWCODE_HOST: HOST,
      SWCODE_PORT: String(port),
      SWCODE_RELEASES_URL: releases.url,
      // Not every machine can make a sandbox: the tests run their commands the same way everywhere, but the one
      // about the sandbox.
      SWCODE_SANDBOX: options.sandbox === true ? 'on' : 'off',
      SWCODE_TOKEN: TOKEN,
      SWCODE_WEB_DIR: WEB_DIR,
      SWCODE_WORKSPACE: workspace,
      ...(chromiumPath === undefined ? {} : { SWCODE_CHROMIUM: chromiumPath }),
    },
  });
  const output: string[] = [];
  daemon.stdout.on('data', (chunk: Buffer) => output.push(chunk.toString('utf8')));
  daemon.stderr.on('data', (chunk: Buffer) => output.push(chunk.toString('utf8')));
  const address = `http://${HOST}:${port}`;
  await untilReady(daemon, address, () => output.join(''));
  const browser = await chromium.launch(chromiumPath === undefined ? {} : { executablePath: chromiumPath });
  return {
    browser,
    events: () => eventsIn(dataDir),
    link: `${address}/?${TOKEN_PARAM}=${TOKEN}`,
    llama,
    program,
    stop: async () => {
      await browser.close();
      await stopDaemon(daemon);
      await llama.stop();
      await releases.stop();
      rmSync(workspace, { force: true, recursive: true });
      rmSync(dataDir, { force: true, recursive: true });
      rmSync(home, { force: true, recursive: true });
    },
    workspace,
  };
}
