import type { FlowGraph } from '@syware/jlib';
import type { SessionEvent } from '../../../shared/protocol/session.schema';
import type { Decision, DecisionMap, DecisionRecord } from '../types/decision-map.types';

/** How much of a message a node's label says before it is cut short. */
const LABEL_LENGTH = 64;
const ELLIPSIS = '…';
/** How far apart the map puts two decisions: enough for a node and the edge between them. */
const NODE_GAP = 136;
const HEADING = 'Decision';
const ICON = 'git-branch';
const TONE = 'accent';

/** What the events say of each call the model made, read once for the whole session. */
interface Lookups {
  /** The choice the call's result recorded, by call id: a call that failed recorded none. */
  records: ReadonlyMap<string, DecisionRecord>;
  /** The reply that made each call, by call id: a fork at the choice starts before that reply. */
  callMessages: ReadonlyMap<string, number>;
}

const oneLine = (text: string): string => text.trim().split('\n')[0]?.trim() ?? '';

/** The first words of a message, on one line, for the label of its node. */
const snippet = (text: string): string => {
  const line = oneLine(text);
  return line.length <= LABEL_LENGTH ? line : `${line.slice(0, LABEL_LENGTH).trimEnd()}${ELLIPSIS}`;
};

/** The choice a call's result recorded, keyed by the call it belongs to: a failed call recorded none. */
const recordOf = (event: SessionEvent): { callId: string; record: DecisionRecord } | undefined => {
  if (event.kind !== 'toolFinished' || event.ok === false) {
    return undefined;
  }
  const display = event.display;
  if (display?.kind !== 'decision') {
    return undefined;
  }
  return {
    callId: event.callId,
    record: { chosen: display.chosen, options: display.options, question: display.question, why: display.why },
  };
};

const recordText = (record: DecisionRecord): string => {
  const lines = [record.question, '', `Chosen: ${record.chosen}`, `Options: ${record.options.join(', ')}`];
  if (record.why !== undefined && record.why !== '') {
    lines.push(`Why: ${record.why}`);
  }
  return lines.join('\n');
};

/** The choice each call recorded, and the reply that made it: the page is sent no reply's own calls. */
function lookupsOf(events: readonly SessionEvent[]): Lookups {
  const records = new Map<string, DecisionRecord>();
  const callMessages = new Map<string, number>();
  let messageIndex = 0;
  for (const [index, event] of events.entries()) {
    if (event.kind === 'assistantMessage') {
      messageIndex = index;
      continue;
    }
    if (event.kind === 'toolCall') {
      callMessages.set(event.callId, messageIndex);
      continue;
    }
    const recorded = recordOf(event);
    if (recorded !== undefined) {
      records.set(recorded.callId, recorded.record);
    }
  }  return { callMessages, records };
}
/** The node of a `decision` tool call, from the choice its result recorded. */
function decisionOf(event: SessionEvent, index: number, lookups: Lookups): Decision | undefined {
  if (event.kind !== 'toolCall' || event.tool !== 'decision') {
    return undefined;
  }
  const record = lookups.records.get(event.callId);
  if (record === undefined) {
    return undefined;
  }
  return {
    at: event.at,
    calls: [],
    cut: lookups.callMessages.get(event.callId) ?? index,
    heading: HEADING,
    icon: ICON,
    id: `decision:${event.callId}`,
    kind: 'decision',
    label: snippet(record.question) || 'A choice',
    line: `Chosen · ${record.chosen}`,
    record,
    text: recordText(record),
    tone: TONE,
  };
}

/** The decisions joined in the order they were made, each a row below the one before. */
const graphOf = (decisions: readonly Decision[]): FlowGraph => ({
  edges: decisions.flatMap((decision, index) => {
    const previous = decisions[index - 1];
    return previous === undefined ? [] : [{ from: previous.id, id: `edge:${decision.id}`, to: decision.id }];
  }),
  nodes: decisions.map((decision, index) => ({
    description: decision.line,
    icon: decision.icon,
    id: decision.id,
    label: decision.label,
    position: { x: 0, y: index * NODE_GAP },
    ...(index === 0 ? { start: true } : {}),
    tone: decision.tone,
  })),
});

/** The session's decisions, in order, and the graph the map draws from them. */
export function decisionMapOf(events: readonly SessionEvent[]): DecisionMap {
  const lookups = lookupsOf(events);
  const decisions = events
    .map((event, index) => decisionOf(event, index, lookups))
    .filter((decision) => decision !== undefined);
  return { decisions, graph: graphOf(decisions) };
}
