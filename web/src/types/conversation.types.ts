import type { HostRef } from '../../../shared/protocol/project.schema';
import type { SessionEvent, SessionSummary, ToolDisplay, ToolName } from '../../../shared/protocol/session.schema';

/** Where a tool call stands: announced, waiting for a person, running, or finished either way. */
export type ToolRowState = 'pending' | 'waiting' | 'queued' | 'running' | 'done' | 'failed';

export type Tone = 'muted' | 'accent' | 'danger';

/** The few words at the end of a tool row, and their colour. */
export interface ToolMeta {
  text: string;
  tone: Tone;
}

/** What a tool row shows once its call finished, set on it as a property. */
export interface ToolResult {
  ok: boolean;
  meta: string | undefined;
  display: ToolDisplay | undefined;
}

export type SessionEventMap = { [Event in SessionEvent as Event['kind']]: Event };

/** One handler per kind of event: a new kind does not compile until the conversation knows how to show it. */
export type SessionEventHandlers = { [Kind in keyof SessionEventMap]: (event: SessionEventMap[Kind]) => void };

/** A project in the side list, with its sessions; a session's folder without a project has no id. */
export interface ProjectGroup {
  projectId: string | undefined;
  /** Where the folder is: this computer, a WSL distribution, an SSH endpoint. */
  host: HostRef;
  name: string;
  path: string;
  sessions: SessionSummary[];
}

/** How a status line reads: its words, a detail under them, and whether things are fine. */
export interface StatusLine {
  text: string;
  detail: string;
  tone: 'ok' | 'problem' | 'quiet';
}

/** A call a subagent made, in a line under the agent call that started it. */
export interface SubagentLine {
  tool: ToolName | undefined;
  summary: string;
  ok: boolean;
  meta: string | undefined;
}

export type AgentState = 'waiting' | 'working' | 'done' | 'failed';

/** A subagent as the side panel lists it: what it was asked, on which model, and what it does now. */
export interface AgentView {
  id: string;
  description: string;
  model: string | undefined;
  state: AgentState;
  activity: string;
}
