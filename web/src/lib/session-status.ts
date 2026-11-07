import type { StatusLook } from '@syware/jlib';
import type { SessionStatus } from '../../../shared/protocol/session.schema';
import type { SessionActivity } from '../types/session-status.types';

const DELETE_HINT = 'Delete this session';
const BUSY_DELETE_HINT = 'Stop this session before deleting it';

/** How each state of a session shows, the same in every view: a new state does not compile until it is here. */
export const SESSION_STATUS_LOOKS: Readonly<Record<SessionStatus, StatusLook>> = {
  idle: { hint: 'Its turn is over: it waits for your next message.', label: 'Idle', tone: 'neutral' },
  waiting: {
    hint: 'It asks you first: an approval, or a question.',
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
