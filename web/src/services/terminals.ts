import { Bus } from '@jadis/core';

import { sameHost } from '../../../shared/hosts/host-key';
import { daemonBus } from './daemon';

import type { Terminal, TerminalState } from '../../../shared/protocol/messages.schema';
import type { HostRef } from '../../../shared/protocol/project.schema';

// The list lives as long as the page: its bus listener never needs to stop.
const forever = new AbortController().signal;

/** The list of terminals as the daemon last gave it: a page opened late reads it here, before any message comes. */
export const terminalListBus = new Bus<{ changed: undefined }>();

const state = { terminals: [] as Terminal[] };

/** Every terminal the daemon runs, on every project. */
export const allTerminals = (): readonly Terminal[] => state.terminals;

// How a terminal stands, as one string: an exit code tells a clean end from a failure.
const stateKey = (state: TerminalState): string => (state.kind === 'exited' ? `exited:${state.code}` : state.kind);

/** What a page shows of a terminal: the list is drawn again only when one of these changes. */
export const terminalKey = (terminal: Terminal): string =>
  `${terminal.id} ${terminal.title} ${stateKey(terminal.state)}`;

/** The terminals of one project's folder, on its host: the ones its sessions show. */
export const terminalsIn = (cwd: string, host: HostRef): Terminal[] =>
  state.terminals.filter((terminal) => terminal.cwd === cwd && sameHost(terminal.host, host));

daemonBus.register(
  'message',
  (message) => {
    if (message.type !== 'terminals') {
      return;
    }
    state.terminals = message.terminals;
    terminalListBus.emit('changed');
  },
  forever
);
