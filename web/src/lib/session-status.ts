import type { StatusLook } from '@syware/jlib';
import type { SessionStatus, SessionSummary } from '../../../shared/protocol/session.schema';
import type { SessionActivity } from '../types/session-status.types';

const DELETE_HINT = 'Delete this session';
const BUSY_DELETE_HINT = 'Stop this session before deleting it';

/** How each state of a session shows, the same in every view: a new state does not compile until it is here. */
export const SESSION_STATUS_LOOKS: Readonly<Record<SessionStatus, StatusLook>> = {
  idle: { hint: 'Its turn is over: it waits for your next message.', label: 'Idle', tone: 'neutral' },
  waiting: {
    hint: 'It needs you: an approval, a question, or a password to type in a terminal.',
    icon: 'circle-alert',
    label: 'Needs you',
    tone: 'warn',
  },
  working: { hint: 'Its turn runs, or its subagents do.', icon: 'loader', label: 'Working', tone: 'accent' },
};

/**
 * A session's state as every view shows it: working while its subagents work in the background, though its own turn
 * ended. Its turn's state is the composer's: a message then starts a turn at once.
 */
export const liveStatus = (session: SessionActivity): SessionStatus =>
  session.status === 'idle' && session.subagents > 0 ? 'working' : session.status;

/** Whether a session can be deleted: the daemon deletes none whose turn runs (waiting for you too) or subagents do. */
export const canDeleteSession = (session: SessionActivity): boolean => liveStatus(session) === 'idle';

/** What a session's delete button says: what it does, or why it cannot yet. */
export const deleteHintOf = (session: SessionActivity): string =>
  canDeleteSession(session) ? DELETE_HINT : BUSY_DELETE_HINT;

/** The icon of a session in the search palette, by its state; the navigation lights its bubble instead. */
export const SESSION_STATUS_ICONS: Readonly<Record<SessionStatus, string>> = {
  idle: 'message',
  waiting: 'circle-alert',
  working: 'loader',
};

/** What waits for the person, in a list of sessions: those that need them. */
export const waitingCount = (sessions: readonly { status: SessionStatus }[]): number =>
  sessions.filter((session) => session.status === 'waiting').length;

// How pressing a state is: a run shows the one of its steps that needs the person most.
const STATUS_PRESSURE: Readonly<Record<SessionStatus, number>> = { idle: 0, waiting: 2, working: 1 };

const pressing = (first: SessionStatus, second: SessionStatus): SessionStatus =>
  STATUS_PRESSURE[second] > STATUS_PRESSURE[first] ? second : first;

const latestOf = (dates: readonly string[]): string => dates.toSorted().at(-1) ?? '';

/** A run's row as its steps make it: their most pressing state, their subagents, and their last activity. */
const withRunState = (run: SessionSummary, steps: readonly SessionSummary[]): SessionSummary => {
  const states = [run, ...steps];
  return {
    ...run,
    status: states.map((state) => liveStatus(state)).reduce(pressing),
    subagents: states.reduce((count, state) => count + state.subagents, 0),
    updatedAt: latestOf(states.map((state) => state.updatedAt)),
  };
};

/**
 * The sessions with each run row showing its steps' state: a run's own turn never runs, so its row would stay idle
 * while a step works, and its last activity is what its steps did.
 */
export const withWorkflowRunStates = (sessions: readonly SessionSummary[]): SessionSummary[] =>
  sessions.map((session) => {
    if (session.workflowRunId === undefined || session.workflowStepId !== undefined) {
      return session;
    }
    const steps = sessions.filter(
      (other) => other.workflowRunId === session.workflowRunId && other.workflowStepId !== undefined
    );
    return steps.length === 0 ? session : withRunState(session, steps);
  });
