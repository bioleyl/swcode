import { Bus } from '@jadis/core';

import { daemon, daemonBus } from './daemon';

import type { SessionMatch } from '../../../shared/protocol/messages.schema';

// The search lives as long as the page: its bus listener never needs to stop.
const forever = new AbortController().signal;
// Fewer letters are in nearly every conversation: they find nothing of use.
const MIN_QUERY_LENGTH = 2;

/** The conversations that say what the palette's search typed, as the daemon answers the latest query. */
export const sessionSearchBus = new Bus<{ found: SessionMatch[] }>();

const state = { query: '' };

/**
 * Asks the daemon for the conversations that say every word of the query; too short a query finds none, at once.
 * Whether the daemon was asked, for the palette to show it looks.
 */
export function searchSessions(query: string): boolean {
  state.query = query.trim();
  if (state.query.length < MIN_QUERY_LENGTH) {
    sessionSearchBus.emit('found', []);
    return false;
  }
  daemon.send({ query: state.query, type: 'searchSessions' });
  return true;
}

daemonBus.register(
  'message',
  (message) => {
    // An answer to a query typed over since is too late.
    if (message.type !== 'sessionMatches' || message.query !== state.query) {
      return;
    }
    sessionSearchBus.emit('found', message.matches);
  },
  forever
);
