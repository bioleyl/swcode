import type { Status } from '../../../shared/protocol/messages.schema';

/**
 * What the app knows about the daemon: not yet asked, answering, refusing the token, or not answering. `refused` and
 * `badAnswer` keep a daemon that is up but unhappy apart from one nobody reaches.
 */
export type DaemonState =
  | { kind: 'unknown' }
  | { kind: 'ready'; status: Status }
  // `pairingFailed`: the page came from a pairing code the daemon refused.
  | { kind: 'locked'; pairingFailed: boolean }
  // The daemon answered with something other than the status: its code, and what it said about itself.
  | { kind: 'refused'; problem: string; status: number }
  // The daemon answered, but not with a status the app can read.
  | { kind: 'badAnswer' }
  | { kind: 'offline' };

export type DaemonStateMap = { [State in DaemonState as State['kind']]: State };
