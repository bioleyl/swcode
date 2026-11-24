import { readFileSync } from 'node:fs';
import { homedir } from 'node:os';
import { join } from 'node:path';
import { Socket } from 'node:net';

import { parseSettingsEnv, portOf } from './lib/settings-env';
import {
  DATA_DIR_ENV,
  DATA_DIR_NAME,
  DEFAULT_PORT,
  PROBE_TIMEOUT_MS,
  SETTINGS_FILE,
  TOKEN_FILE,
} from './swcode.constants';

import type { DaemonProbe, DaemonTarget } from './types/daemon.types';

// The daemon answers on this computer: the loopback, as the link the daemon prints uses.
const LOCAL_HOST = '127.0.0.1';
const SCHEME = 'http://';
const PORT_SEPARATOR = ':';
const UTF8 = 'utf8';
const FALLBACK_TEXT = '';

/** The data folder: `SWCODE_DATA_DIR`, else `~/.swcode` — the same order daemon/config reads its settings in. */
function dataDirOf(environment: NodeJS.ProcessEnv): string {
  const given = environment[DATA_DIR_ENV];
  return given && given !== '' ? given : join(homedir(), DATA_DIR_NAME);
}

// A data folder of a fresh computer holds nothing: a file that is not there reads as empty.
function readText(path: string): string {
  try {
    return readFileSync(path, UTF8);
  } catch {
    return FALLBACK_TEXT;
  }
}

/**
 * The daemon on this machine: its address from the settings file's port, and the access token kept beside it.
 * Nothing is paired: the extension talks to the daemon of the computer it runs on.
 */
export function readTarget(environment: NodeJS.ProcessEnv = process.env): DaemonTarget {
  const dir = dataDirOf(environment);
  const port = portOf(parseSettingsEnv(readText(join(dir, SETTINGS_FILE))));
  return { daemon: `${SCHEME}${LOCAL_HOST}:${port}`, token: readText(join(dir, TOKEN_FILE)).trim() };
}

/** The address as people read it, host and port, for the status bar's tooltip. */
export function addressOf(target: DaemonTarget): string {
  return target.daemon.slice(SCHEME.length);
}

/** The port of the target's address, for the socket probe. */
function portOfTarget(target: DaemonTarget): number {
  const separator = addressOf(target).indexOf(PORT_SEPARATOR);
  return separator < 0 ? DEFAULT_PORT : Number(addressOf(target).slice(separator + 1));
}

/**
 * Asks the daemon's port, once: the socket's answer, or the failure that stopped it. Nothing is sent, so the
 * answer comes as soon as the port accepts.
 */
export function probe(target: DaemonTarget, timeoutMs: number = PROBE_TIMEOUT_MS): Promise<DaemonProbe> {
  const socket = new Socket();
  socket.setTimeout(timeoutMs);
  return new Promise<DaemonProbe>((resolve) => {
    const settle = (probeResult: DaemonProbe) => {
      socket.destroy();
      resolve(probeResult);
    };
    socket.connect(portOfTarget(target), LOCAL_HOST);
    socket.once('connect', () => settle({ kind: 'connected' }));
    socket.once('timeout', () => settle({ kind: 'unreachable', problem: 'timed out' }));
    socket.once('error', (error: Error) => settle({ kind: 'unreachable', problem: error.message }));
  });
}
