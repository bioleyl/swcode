import type { FlowGraph, Tone } from '@syware/jlib';
import type { SessionEvent } from '../../../shared/protocol/session.schema';

/** The events that hold a decision, each kind with the fields the map reads. */
export type UserMessageEvent = Extract<SessionEvent, { kind: 'userMessage' }>;
export type AssistantMessageEvent = Extract<SessionEvent, { kind: 'assistantMessage' }>;
export type QuestionAskedEvent = Extract<SessionEvent, { kind: 'questionAsked' }>;
export type PermissionRequestedEvent = Extract<SessionEvent, { kind: 'permissionRequested' }>;
export type ToolCallEvent = Extract<SessionEvent, { kind: 'toolCall' }>;

/** The kinds of decision a session holds: the person's, the model's, a choice it recorded, and the two it can wait on. */
export type DecisionKind = 'you' | 'model' | 'decision' | 'question' | 'permission';

/** A choice the model recorded with the decision tool: what it weighed, what it chose, and why. */
export interface DecisionRecord {
  question: string;
  options: string[];
  chosen: string;
  why?: string;
}

/** A tool call the model made in a decision, as the map lists it. */
export interface DecisionCall {
  name: string;
  summary: string;
  ok: boolean | undefined;
  meta: string | undefined;
}

/** A question the agent asked, and the answer the person gave (empty while none came). */
export interface DecisionQuestion {
  answer: string;
  question: string;
}

/** One decision of a session: where it stands in the map, and what it decided. */
export interface Decision {
  id: string;
  kind: DecisionKind;
  at: string;
  /** How many of the session's events come before this decision: a fork at it keeps those. */
  cut: number;
  /** What the detail card calls this decision: "You", "Model", "Decision", "Question" or "Permission". */
  heading: string;
  label: string;
  line: string;
  text: string;
  calls: DecisionCall[];
  icon: string;
  tone: Tone;
  /** The questions and answers of a question the person answered: they can answer them again in a fork. */
  questions?: DecisionQuestion[];
  /** The choice the model recorded: the person can choose another option in a fork. */
  record?: DecisionRecord;
}

/** The decisions of a session, and the graph the map draws from them. */
export interface DecisionMap {
  decisions: Decision[];
  graph: FlowGraph;
}
