import type { Status } from '../../../shared/protocol/messages.schema';
import type { SourceStatus } from '../../../shared/protocol/models.schema';
import type { StatusLine } from '../types/conversation.types';
import type { DaemonState, DaemonStateMap } from '../types/daemon.types';

const PROTOCOL = /^https?:\/\//;

/** An llm-server's address without its protocol: `gmktec:8080`. */
export const hostOf = (url: string): string => url.replace(PROTOCOL, '');

const hostsOf = (sources: readonly SourceStatus[]): string =>
  sources.map((source) => hostOf(source.url)).join(', ');

/** Whether every llm-server answers; one that does not is named, when there are several. */
const llamaLine = (status: Status): StatusLine => {
  const sources = status.llama.sources;
  const silent = sources.filter((source) => !source.reachable);
  const first = silent[0];
  if (first === undefined) {
    return { detail: hostsOf(sources), text: 'llm-server connected', tone: 'ok' };
  }
  const named = silent.length === 1 && sources.length > 1;
  return {
    detail: `Nothing answers at ${hostsOf(silent)}`,
    text: named ? `${first.name} unreachable` : 'llm-server unreachable',
    tone: 'problem',
  };
};

const LINES: { [Kind in keyof DaemonStateMap]: (state: DaemonStateMap[Kind]) => StatusLine } = {
  locked: ({ pairingFailed }) =>
    pairingFailed
      ? {
          detail: 'Scan the QR code again: its code works once, for a minute.',
          text: 'This pairing code expired',
          tone: 'problem',
        }
      : {
          detail: 'Open the link the daemon printed when it started, or pair with the QR code.',
          text: 'The access token is missing',
          tone: 'problem',
        },
  offline: () => ({ detail: 'Is it running?', text: 'The daemon does not answer', tone: 'problem' }),
  ready: ({ status }) => llamaLine(status),
  unknown: () => ({ detail: '', text: 'Connecting…', tone: 'quiet' }),
};

const lineFor = <Kind extends keyof DaemonStateMap>(kind: Kind, state: DaemonStateMap[Kind]): StatusLine =>
  LINES[kind](state);

/** Whether the daemon and llm-server answer: the side panel's, and the alert over every page on a problem. */
export const daemonLine = (state: DaemonState): StatusLine => lineFor(state.kind, state);
