import type { ToolName } from '../../../shared/protocol/session.schema';

/**
 * What the agent is doing while its turn runs: waiting on the model between steps, thinking, writing its text,
 * writing a call to a tool, or running one.
 */
export type Activity =
  | { kind: 'working' }
  | { kind: 'thinking' }
  | { kind: 'writing' }
  | { kind: 'calling'; tool: ToolName }
  | { kind: 'running'; tool: ToolName | undefined; summary: string };

export type ActivityMap = { [Each in Activity as Each['kind']]: Each };

/** One line per kind of activity: a new kind does not compile until it has its words. */
export type ActivityLines = { [Kind in keyof ActivityMap]: (activity: ActivityMap[Kind]) => string };
