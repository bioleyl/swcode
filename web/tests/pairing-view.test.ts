import { describe, expect, it } from 'vitest';

import { listensHereAlone, reachableBaseOf } from '../src/lib/pairing-view';

import type { DaemonState } from '../src/types/daemon.types';

type Status = Extract<DaemonState, { kind: 'ready' }>['status'];

const LOCAL_PAGE = 'http://localhost:4500';

// The addresses are all this file is about: the rest of a status is the daemon's to say.
const ready = (addresses: Pick<Status, 'lanUrls' | 'publicUrl'>): DaemonState =>
  ({ kind: 'ready', status: addresses }) as DaemonState;

describe('whether another device can reach swcode', () => {
  it('cannot when the daemon has no address but this computer’s own', () => {
    expect(listensHereAlone(ready({}), LOCAL_PAGE)).toBe(true);
    expect(listensHereAlone(ready({ lanUrls: [] }), 'http://127.0.0.1:4500')).toBe(true);
  });

  it('can once it has an address on the network, or one it was given', () => {
    expect(listensHereAlone(ready({ lanUrls: ['http://192.168.1.121:4500'] }), LOCAL_PAGE)).toBe(false);
    expect(listensHereAlone(ready({ publicUrl: 'https://marvin.tail1234.ts.net' }), LOCAL_PAGE)).toBe(false);
  });

  it('can when the page itself came from another address: something reached it', () => {
    expect(listensHereAlone(ready({}), 'https://swcode.example.com')).toBe(false);
  });

  it('is not said before the daemon answered', () => {
    expect(listensHereAlone({ kind: 'unknown' }, LOCAL_PAGE)).toBe(false);
  });
});

describe('the address a phone opens', () => {
  it('is the one the daemon was given, else its first on the network, else the page’s', () => {
    const addresses = { lanUrls: ['http://192.168.1.121:4500'], publicUrl: 'https://marvin.tail1234.ts.net' };
    expect(reachableBaseOf(ready(addresses), LOCAL_PAGE)).toBe('https://marvin.tail1234.ts.net');
    expect(reachableBaseOf(ready({ lanUrls: addresses.lanUrls }), LOCAL_PAGE)).toBe('http://192.168.1.121:4500');
    expect(reachableBaseOf(ready({}), LOCAL_PAGE)).toBe(LOCAL_PAGE);
  });
});
