import { mkdtempSync, readFileSync, writeFileSync } from 'node:fs';
import { createServer } from 'node:net';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { describe, expect, it } from 'vitest';

import { addressOf, probe, readTarget } from '../src/daemon';
import {
  ADD_TESTS_COMMAND,
  ADD_TESTS_TITLE,
  ASK_COMMAND,
  ASK_TITLE,
  EXPLAIN_COMMAND,
  EXPLAIN_TITLE,
  FIX_PROBLEMS_COMMAND,
  FIX_PROBLEMS_TITLE,
  REFACTOR_COMMAND,
  REFACTOR_TITLE,
  VIEW_CONTAINER_ID,
  VIEW_ID,
} from '../src/swcode.constants';

const manifest = JSON.parse(readFileSync(new URL('../package.json', import.meta.url), 'utf8')) as {
  contributes: {
    commands: { command: string; title: string }[];
    menus: Record<string, { command: string }[]>;
    views: Record<string, { id: string; type: string }[]>;
    viewsContainers: { activitybar: { id: string }[] };
  };
};

function dataFolder(settings: string, token: string): string {
  const dir = mkdtempSync(join(tmpdir(), 'swcode-vscode-'));
  writeFileSync(join(dir, 'settings.env'), settings);
  writeFileSync(join(dir, 'token'), `${token}\n`);
  return dir;
}

describe('the daemon of this computer', () => {
  it('reads its port from the settings and its token beside them', () => {
    const dir = dataFolder('SWCODE_PORT=4711\n', 'aBc\n');
    expect(readTarget({ SWCODE_DATA_DIR: dir })).toEqual({
      daemon: 'http://127.0.0.1:4711',
      token: 'aBc',
    });
  });

  it('falls back on the default port and an empty token for a fresh data folder', () => {
    expect(readTarget({ SWCODE_DATA_DIR: join(tmpdir(), 'swcode-absent') })).toEqual({
      daemon: 'http://127.0.0.1:4500',
      token: '',
    });
  });

  it('names the address the status bar shows', () => {
    expect(addressOf({ daemon: 'http://127.0.0.1:4711', token: '' })).toBe('127.0.0.1:4711');
  });
});

describe('the daemon’s port', () => {
  it('answers connected when the port accepts', async () => {
    const server = createServer().listen(0);
    const port = (server.address() as { port: number }).port;
    const state = await probe({ daemon: `http://127.0.0.1:${port}`, token: '' });
    server.close();
    expect(state).toEqual({ kind: 'connected' });
  });

  it('answers unreachable when the port is closed', async () => {
    const server = createServer().listen(0);
    const port = (server.address() as { port: number }).port;
    // The kernel lets the port accept for a moment after the listener closed: the probe asks once it is free.
    await new Promise((closed) => server.close(() => setTimeout(closed, 10)));
    const state = await probe({ daemon: `http://127.0.0.1:${port}`, token: '' }, 200);
    expect(state.kind).toBe('unreachable');
  });
});

describe('the manifest', () => {
  it('gives the view container and the view the ids the extension registers', () => {
    expect(manifest.contributes.viewsContainers.activitybar[0]?.id).toBe(VIEW_CONTAINER_ID);
    expect(manifest.contributes.views[VIEW_CONTAINER_ID]?.[0]).toMatchObject({
      id: VIEW_ID,
      type: 'webview',
    });
  });

  it('contributes “Ask swcode” to the palette and the editor’s context menu', () => {
    expect(manifest.contributes.commands).toContainEqual({
      category: 'swcode',
      command: ASK_COMMAND,
      title: ASK_TITLE,
    });
    expect(manifest.contributes.menus['editor/context']).toContainEqual({ command: ASK_COMMAND, group: 'swcode@1' });
  });

  it('contributes the four refactor commands after it, in the palette and the editor’s context menu', () => {
    expect(manifest.contributes.commands).toEqual(
      expect.arrayContaining([
        { category: 'swcode', command: REFACTOR_COMMAND, title: REFACTOR_TITLE },
        { category: 'swcode', command: EXPLAIN_COMMAND, title: EXPLAIN_TITLE },
        { category: 'swcode', command: FIX_PROBLEMS_COMMAND, title: FIX_PROBLEMS_TITLE },
        { category: 'swcode', command: ADD_TESTS_COMMAND, title: ADD_TESTS_TITLE },
      ]),
    );
    expect(manifest.contributes.menus['editor/context']).toEqual(
      expect.arrayContaining([
        { command: REFACTOR_COMMAND, group: 'swcode@2' },
        { command: EXPLAIN_COMMAND, group: 'swcode@3' },
        { command: FIX_PROBLEMS_COMMAND, group: 'swcode@4' },
        { command: ADD_TESTS_COMMAND, group: 'swcode@5' },
      ]),
    );
  });
});
