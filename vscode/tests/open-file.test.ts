import { beforeEach, describe, expect, it, vi } from 'vitest';

import { openInEditor, openRequestFrom, rangeOf } from '../src/open-file';

import type { OpenRequest } from '../../web/src/types/ide-open.types';

/** The pieces of VS Code the module uses: the mock keeps what the extension writes to them. */
const host = vi.hoisted<{
  setStatusBarMessage: ReturnType<typeof vi.fn>;
  showTextDocument: ReturnType<typeof vi.fn>;
  openTextDocument: ReturnType<typeof vi.fn>;
  uriFile: ReturnType<typeof vi.fn>;
}>(() => ({
  openTextDocument: vi.fn(),
  setStatusBarMessage: vi.fn(),
  showTextDocument: vi.fn(),
  uriFile: vi.fn((path: string) => ({ fsPath: path, scheme: 'file' })),
}));

vi.mock('vscode', () => ({
  Range: class {
    constructor(
      public startLine: number,
      public startColumn: number,
      public endLine: number,
      public endColumn: number
    ) {}
  },
  Uri: { file: host.uriFile },
  window: host,
  workspace: host,
}));

const FILE = '/home/dev/app/src/main.ts';

beforeEach(() => {
  vi.clearAllMocks();
  host.openTextDocument.mockResolvedValue({});
  host.showTextDocument.mockResolvedValue({});
});

describe('the open request the webview posts', () => {
  it('keeps the file and the lines to show', () => {
    expect(openRequestFrom({ endLine: 14, path: FILE, startLine: 12, type: 'open' })).toEqual({
      endLine: 14,
      path: FILE,
      startLine: 12,
      type: 'open',
    } satisfies OpenRequest);
  });

  it('keeps a request that names no lines, and ignores what is not one', () => {
    expect(openRequestFrom({ path: FILE, type: 'open' })).toEqual({ path: FILE, type: 'open' });
    expect(openRequestFrom({ path: '', type: 'open' })).toBeUndefined();
    expect(openRequestFrom({ text: 'why this?', type: 'prompt' })).toBeUndefined();
    expect(openRequestFrom(undefined)).toBeUndefined();
  });
});

describe('the range the editor shows', () => {
  it('turns the 1-based lines into a 0-based range, at the first column', () => {
    expect(rangeOf({ path: FILE, startLine: 12, endLine: 14, type: 'open' })).toEqual({
      endColumn: 0,
      endLine: 13,
      startColumn: 0,
      startLine: 11,
    });
  });

  it('ends on the line it starts on when the request names one', () => {
    expect(rangeOf({ path: FILE, startLine: 3, type: 'open' })).toEqual({
      endColumn: 0,
      endLine: 2,
      startColumn: 0,
      startLine: 2,
    });
  });

  it('gives no range at all when the request names none: the file opens whole', () => {
    expect(rangeOf({ path: FILE, type: 'open' })).toBeUndefined();
  });
});

describe('opening the file in the editor', () => {
  it('opens the path in a preview tab, at the range the request names', async () => {
    openInEditor({ endLine: 14, path: FILE, startLine: 12, type: 'open' });
    await Promise.resolve();
    expect(host.uriFile).toHaveBeenCalledWith(FILE);
    expect(host.openTextDocument).toHaveBeenCalledWith({ fsPath: FILE, scheme: 'file' });
    expect(host.showTextDocument).toHaveBeenCalledWith(
      {},
      { preview: true, selection: { endColumn: 0, endLine: 13, startColumn: 0, startLine: 11 } }
    );
  });

  it('says in the status bar when the path cannot be opened, and keeps going', async () => {
    host.openTextDocument.mockRejectedValue(new Error('ENOENT'));
    openInEditor({ path: FILE, type: 'open' });
    await Promise.resolve();
    expect(host.showTextDocument).not.toHaveBeenCalled();
    expect(host.setStatusBarMessage).toHaveBeenCalled();
  });

  it('opens nothing for a message that is not an open request', () => {
    openInEditor({ text: 'why this?', type: 'prompt' });
    expect(host.openTextDocument).not.toHaveBeenCalled();
  });
});
