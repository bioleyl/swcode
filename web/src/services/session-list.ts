import { Bus } from '@jadis/core';

import { daemonBus } from './daemon';

import type { SessionSummary } from '../../../shared/protocol/session.schema';

// The list lives as long as the page: its bus listener never needs to stop.
const forever = new AbortController().signal;

/** The sessions as the daemon last listed them; `created` fires for a session this page asked for. */
export const sessionListBus = new Bus<{ changed: undefined; created: SessionSummary }>();

const state = { sessions: [] as SessionSummary[] };

export const allSessions = (): readonly SessionSummary[] => state.sessions;

export const findSession = (id: string): SessionSummary | undefined =>
  state.sessions.find((session) => session.id === id);

daemonBus.register(
  'message',
  (message) => {
    if (message.type === 'sessions') {
      state.sessions = message.sessions;
      sessionListBus.emit('changed');
      return;
    }
    if (message.type === 'sessionCreated') {
      sessionListBus.emit('created', message.session);
    }
  },
  forever
);
