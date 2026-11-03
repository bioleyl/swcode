import type { ToolMeta, ToolResult, ToolRowState } from '../types/conversation.types';

const MINUS = '−';

/** A finished call's few words: the diff's size for a change to a file, otherwise what the tool said. */
const finishedMeta = (result: ToolResult | undefined, fallback: string): string => {
  const display = result?.display;
  if (display?.kind === 'diff') {
    return `+${display.added} ${MINUS}${display.removed}`;
  }
  return result?.meta ?? fallback;
};

const META: Readonly<Record<ToolRowState, (result: ToolResult | undefined) => ToolMeta>> = {
  done: (result) => ({
    text: finishedMeta(result, 'done'),
    tone: result?.display?.kind === 'diff' ? 'accent' : 'muted',
  }),
  failed: (result) => ({ text: result?.meta ?? 'failed', tone: 'danger' }),
  pending: () => ({ text: '', tone: 'muted' }),
  running: () => ({ text: 'Running', tone: 'accent' }),
  waiting: () => ({ text: 'Waiting for you', tone: 'accent' }),
};

export const toolMeta = (state: ToolRowState, result: ToolResult | undefined): ToolMeta => META[state](result);
