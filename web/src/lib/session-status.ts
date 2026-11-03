import type { StatusLook } from '@syware/jlib';
import type { SessionStatus } from '../../../shared/protocol/session.schema';

/** How each state of a session shows, the same in every view: a new state does not compile until it is here. */
export const SESSION_STATUS_LOOKS: Readonly<Record<SessionStatus, StatusLook>> = {
  idle: { hint: 'Its turn is over: it waits for your next message.', label: 'Idle', tone: 'neutral' },
  waiting: {
    hint: 'It asks you first: an approval, or a question.',
    icon: 'circle-alert',
    label: 'Needs you',
    tone: 'warn',
  },
  working: { hint: 'Its turn runs.', icon: 'loader', label: 'Working', tone: 'accent' },
};

/** The icon of a session in the navigation, by its state. */
export const SESSION_STATUS_ICONS: Readonly<Record<SessionStatus, string>> = {
  idle: 'message',
  waiting: 'circle-alert',
  working: 'loader',
};

/** What waits for the person, in a list of sessions: those that need them. */
export const waitingCount = (sessions: readonly { status: SessionStatus }[]): number =>
  sessions.filter((session) => session.status === 'waiting').length;
