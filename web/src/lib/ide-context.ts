import type { IdeContext, IdeDiagnostic } from '../types/ide-context.types';

/** The shortest fence markdown understands. */
const MIN_FENCE = 3;
/** A prompt stays light: the selection and the problems are cut to these. */
const MAX_SELECTION_LINES = 200;
const MAX_DIAGNOSTICS = 30;

/**
 * The whole context as one fenced block, so the conversation shows it as a single piece of code: the file, the
 * lines selected, the code itself, then the problems the host's language server found.
 */
export function contextBlock(context: IdeContext): string {
  const body = [
    `File: ${context.path}`,
    `Selection: ${selectionRange(context)}`,
    `Selected code (${context.language}):`,
    ...selectionLines(context.selection.text),
    ...diagnosticLines(context.diagnostics),
  ];
  const fence = fenceFor(body.join('\n'));
  return `${fence}\n${body.join('\n')}\n${fence}`;
}

/** `12:5-14:3`, the caret's own range when nothing is selected. */
function selectionRange(context: IdeContext): string {
  const { startColumn, startLine, endColumn, endLine } = context.selection;
  const start = `${startLine}:${startColumn}`;
  return startLine === endLine && startColumn === endColumn ? start : `${start}-${endLine}:${endColumn}`;
}

/** The selected code, at most {@link MAX_SELECTION_LINES} lines. */
function selectionLines(text: string): string[] {
  return text === '' ? [''] : text.split('\n').slice(0, MAX_SELECTION_LINES);
}

/** The problems, one per line: `12:5 error 'x' is never read (eslint/no-unused-vars)`, at most 30 of them. */
function diagnosticLines(diagnostics: readonly IdeDiagnostic[]): string[] {
  if (diagnostics.length === 0) {
    return [];
  }
  return ['Problems:', ...diagnostics.slice(0, MAX_DIAGNOSTICS).map(diagnosticLine)];
}

function diagnosticLine(diagnostic: IdeDiagnostic): string {
  return `${diagnostic.line}:${diagnostic.column} ${diagnostic.severity} ${diagnostic.message}${ruleOf(diagnostic)}`;
}

function ruleOf(diagnostic: IdeDiagnostic): string {
  const source = diagnostic.source.trim();
  const code = diagnostic.code.trim();
  const rule = [source, code].filter((part) => part !== '').join('/');
  return rule === '' ? '' : ` (${rule})`;
}

/**
 * A fence longer than the longest run of backticks in the code, so a code block inside the selection cannot close
 * the block early.
 */
const longestBacktickRun = (code: string): number =>
  (code.match(/`+/g) ?? []).reduce((longest, run) => Math.max(longest, run.length), 0);

function fenceFor(code: string): string {
  return '`'.repeat(Math.max(MIN_FENCE, longestBacktickRun(code) + 1));
}
