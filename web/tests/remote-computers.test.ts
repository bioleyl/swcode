import { describe, expect, it } from 'vitest';

import { BAD_PORT, NO_ADDRESS, NOT_SECURE, pairingOrigin } from '../src/lib/remote-computers';

const HTTPS = 'https:';
const HTTP = 'http:';

describe('pairingOrigin', () => {
  it('gives an address without a scheme the page’s own, and a plain one without a port swcode’s 4500', () => {
    const plain = pairingOrigin('gmktec', '', HTTP);
    expect(plain.ok && plain.answer).toBe('http://gmktec:4500');
    const withPort = pairingOrigin('192.168.1.20:4600', '', HTTP);
    expect(withPort.ok && withPort.answer).toBe('http://192.168.1.20:4600');
    const secure = pairingOrigin('gmktec.tail.ts.net', '', HTTPS);
    expect(secure.ok && secure.answer).toBe('https://gmktec.tail.ts.net');
  });

  it('takes the port typed beside the address, over the link’s own', () => {
    const typed = pairingOrigin('gmktec', '8080', HTTP);
    expect(typed.ok && typed.answer).toBe('http://gmktec:8080');
    const secure = pairingOrigin('https://gmktec.tail.ts.net', '4500', HTTPS);
    expect(secure.ok && secure.answer).toBe('https://gmktec.tail.ts.net:4500');
  });

  it('keeps only the address: a path, a query or a token never reaches it', () => {
    const link = pairingOrigin('http://gmktec:4500/?token=secret&pair=old', '', HTTP);
    expect(link.ok && link.answer).toBe('http://gmktec:4500');
  });

  it('refuses nothing typed, and an address that is none', () => {
    expect(pairingOrigin('  ', '', HTTP)).toEqual({ ok: false, problem: NO_ADDRESS });
    expect(pairingOrigin('http://', '', HTTP)).toEqual({ ok: false, problem: NO_ADDRESS });
    expect(pairingOrigin('ftp://gmktec', '', HTTP)).toEqual({ ok: false, problem: NO_ADDRESS });
  });

  it('refuses a port that is not one', () => {
    expect(pairingOrigin('gmktec', 'many', HTTP)).toEqual({ ok: false, problem: BAD_PORT });
    expect(pairingOrigin('gmktec', '70000', HTTP)).toEqual({ ok: false, problem: BAD_PORT });
  });

  it('refuses a plain address from a page that came over HTTPS, which could not talk to it', () => {
    expect(pairingOrigin('http://gmktec:4500', '', HTTPS)).toEqual({ ok: false, problem: NOT_SECURE });
    expect(pairingOrigin('http://gmktec:4500', '', HTTP).ok).toBe(true);
  });
});
