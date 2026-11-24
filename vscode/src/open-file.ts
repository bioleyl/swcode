import { OPEN_MESSAGE, OPEN_MESSAGE_FAILURE, OPEN_MESSAGE_TIMEOUT_MS } from './swcode.constants';

import { Range, Uri, window, workspace } from 'vscode';

import type { OpenRequest } from '../../web/src/types/ide-open.types';

const isRecord = (value: unknown): value is Record<string, unknown> => typeof value === 'object' && value !== null;

const lineOf = (value: unknown): number | undefined => (typeof value === 'number' ? value : undefined);

/**
 * The bundle's open request, as the extension reads it; undefined for anything else the webview posts.
 *
 * `data` is the message the webview was given, so anything can come in.
 */
export function openRequestFrom(data: unknown): OpenRequest | undefined {
  if (!isRecord(data) || data.type !== OPEN_MESSAGE) {
    return undefined;
  }
  const path = typeof data.path === 'string' ? data.path : '';
  if (path === '') {
    return undefined;
  }
  return { endLine: lineOf(data.endLine), path, startLine: lineOf(data.startLine), type: OPEN_MESSAGE };
}

/** The 1-based lines of the request, as the editor's 0-based Range; a request with none opens the file whole. */
export function rangeOf(request: OpenRequest): Range | undefined {
  const startLine = request.startLine;
  if (startLine === undefined) {
    return undefined;
  }
  return new Range(startLine - 1, 0, (request.endLine ?? startLine) - 1, 0);
}

/**
 * Opens the file the bundle asked for, in a preview tab, at the range it names. A path the editor cannot open
 * says so in the status bar, and the panel keeps working.
 */
export function openInEditor(data: unknown): void {
  const request = openRequestFrom(data);
  if (request === undefined) {
    return;
  }
  const uri = Uri.file(request.path);
  workspace.openTextDocument(uri).then(
    (document) => void window.showTextDocument(document, { preview: true, selection: rangeOf(request) }),
    () => window.setStatusBarMessage(OPEN_MESSAGE_FAILURE, OPEN_MESSAGE_TIMEOUT_MS)
  );
}
