import type { Status } from '../../../shared/protocol/messages.schema';

/** What the app knows about the daemon: not yet asked, answering, refusing the token, or out of reach. */
export type DaemonState =
  | { kind: 'unknown' }
  | { kind: 'ready'; status: Status }
  // `pairingFailed`: the page came from a pairing code the daemon refused.
  | { kind: 'locked'; pairingFailed: boolean }
  | { kind: 'offline' };

export type DaemonStateMap = { [State in DaemonState as State['kind']]: State };
