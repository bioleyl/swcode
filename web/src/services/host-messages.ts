import { Bus } from '@jadis/core';

import type { IdeContext, IdeDiagnostic, IdeSelection } from '../types/ide-context.types';
import type { OpenRequest } from '../types/ide-open.types';

/**
 * What the host (the VS Code extension's webview) sends into the bundle's page.
 *
 * A prompt carries `includeProblems` when its command names it (the extension's "Fix problems"); left out, the
 * composer keeps the state its "Include problems" toggle is in.
 */
export type HostMessage =
  | { kind: 'context'; context: IdeContext }
  | { kind: 'prompt'; text: string; includeProblems?: boolean };

export const hostMessageBus = new Bus<{ message: HostMessage }>();

const CONTEXT_MESSAGE = 'context';
const PROMPT_MESSAGE = 'prompt';
/** The message the bundle posts back to the host, asking it to open a file at a place. */
export const OPEN_MESSAGE = 'open';
const MESSAGE_FIELD = 'type';
const CONTEXT_FIELD = 'context';
const TEXT_FIELD = 'text';
const INCLUDE_PROBLEMS_FIELD = 'includeProblems';
const PATH_FIELD = 'path';
const LANGUAGE_FIELD = 'language';
const SELECTION_FIELD = 'selection';
const DIAGNOSTICS_FIELD = 'diagnostics';
const START_LINE_FIELD = 'startLine';
const START_COLUMN_FIELD = 'startColumn';
const END_LINE_FIELD = 'endLine';
const END_COLUMN_FIELD = 'endColumn';
const LINE_FIELD = 'line';
const COLUMN_FIELD = 'column';
const SEVERITY_FIELD = 'severity';
const MESSAGE_FIELD_TEXT = 'message';
const SOURCE_FIELD = 'source';
const CODE_FIELD = 'code';

const isRecord = (value: unknown): value is Record<string, unknown> => typeof value === 'object' && value !== null;

const textOf = (value: unknown): string => (typeof value === 'string' ? value : '');

const numberOf = (value: unknown): number => (typeof value === 'number' ? value : 1);

/** The flag the host set for this prompt; undefined when it set none, which leaves the composer's own choice. */
const flagOf = (value: unknown): boolean | undefined => (typeof value === 'boolean' ? value : undefined);

function selectionOf(value: unknown): IdeSelection {
  const fields = isRecord(value) ? value : {};
  return {
    endColumn: numberOf(fields[END_COLUMN_FIELD]),
    endLine: numberOf(fields[END_LINE_FIELD]),
    startColumn: numberOf(fields[START_COLUMN_FIELD]),
    startLine: numberOf(fields[START_LINE_FIELD]),
    text: textOf(fields[TEXT_FIELD]),
  };
}

function diagnosticOf(value: unknown): IdeDiagnostic {
  const fields = isRecord(value) ? value : {};
  return {
    code: textOf(fields[CODE_FIELD]),
    column: numberOf(fields[COLUMN_FIELD]),
    line: numberOf(fields[LINE_FIELD]),
    message: textOf(fields[MESSAGE_FIELD_TEXT]),
    severity: textOf(fields[SEVERITY_FIELD]),
    source: textOf(fields[SOURCE_FIELD]),
  };
}

function contextOf(value: unknown): IdeContext {
  const fields = isRecord(value) ? value : {};
  const diagnostics = Array.isArray(fields[DIAGNOSTICS_FIELD]) ? fields[DIAGNOSTICS_FIELD] : [];
  return {
    diagnostics: diagnostics.map(diagnosticOf),
    language: textOf(fields[LANGUAGE_FIELD]),
    path: textOf(fields[PATH_FIELD]),
    selection: selectionOf(fields[SELECTION_FIELD]),
  };
}

/**
 * A message the host posted, as the bundle understands it; undefined for anything else the page is sent.
 *
 * `data` is the `MessageEvent`'s data: the object the host's `postMessage` was given, so anything can come in.
 */
export function hostMessageFrom(data: unknown): HostMessage | undefined {
  if (!isRecord(data)) {
    return undefined;
  }
  if (data[MESSAGE_FIELD] === CONTEXT_MESSAGE) {
    return { context: contextOf(data[CONTEXT_FIELD]), kind: CONTEXT_MESSAGE };
  }
  if (data[MESSAGE_FIELD] === PROMPT_MESSAGE) {
    return {
      includeProblems: flagOf(data[INCLUDE_PROBLEMS_FIELD]),
      kind: PROMPT_MESSAGE,
      text: textOf(data[TEXT_FIELD]),
    };
  }
  return undefined;
}

/**
 * Asks the host to open a file, at a range of lines when it names one (1-based, as the editor counts them).
 * The page's parent is the host's webview; a page of its own is its own parent, so the message goes nowhere.
 */
export function requestOpenInHost(path: string, startLine?: number, endLine?: number): void {
  const request: OpenRequest = {
    [END_LINE_FIELD]: endLine,
    [MESSAGE_FIELD]: OPEN_MESSAGE,
    [PATH_FIELD]: path,
    [START_LINE_FIELD]: startLine,
  };
  window.parent.postMessage(request, '*');
}

// The page lives as long as the bundle: the host's messages keep coming, so the listener never needs to stop.
const forever = new AbortController().signal;

/** Starts listening for the host's messages, and hands them to the pages through the bus. */
export function watchHostMessages(): void {
  const listen = (event: MessageEvent): void => {
    const message = hostMessageFrom(event.data);
    if (message !== undefined) {
      hostMessageBus.emit('message', message);
    }
  };
  window.addEventListener('message', listen, { signal: forever });
}
