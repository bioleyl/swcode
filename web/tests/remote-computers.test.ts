import { describe, expect, it } from 'vitest';

import { NO_ADDRESS, NO_CODE, NOT_SECURE, pairingTarget, TOKEN_LINK } from '../src/lib/remote-computers';

const HTTPS = 'https:';
const HTTP = 'http:';

describe('pairingTarget', () => {
  it('reads the address and the code from the link under a QR code', () => {
    expect(pairingTarget(' https://gmktec.tail.ts.net/?pair=abc123 ', '', HTTPS)).toEqual({
      answer: { code: 'abc123', origin: 'https://gmktec.tail.ts.net' },
      ok: true,
    });
  });

  it('takes the code typed beside an address, the link’s own first', () => {
    expect(pairingTarget('https://gmktec.tail.ts.net', ' typed ', HTTPS)).toEqual({
      answer: { code: 'typed', origin: 'https://gmktec.tail.ts.net' },
      ok: true,
    });
    const both = pairingTarget('https://gmktec.tail.ts.net/?pair=linked', 'typed', HTTPS);
    expect(both.ok && both.answer.code).toBe('linked');
  });

  it('gives an address without a scheme the page’s own, and a plain one without a port swcode’s', () => {
    const plain = pairingTarget('gmktec', 'code', HTTP);
    expect(plain.ok && plain.answer.origin).toBe('http://gmktec:4500');
    const withPort = pairingTarget('192.168.1.20:4600', 'code', HTTP);
    expect(withPort.ok && withPort.answer.origin).toBe('http://192.168.1.20:4600');
    const secure = pairingTarget('gmktec.tail.ts.net', 'code', HTTPS);
    expect(secure.ok && secure.answer.origin).toBe('https://gmktec.tail.ts.net');
  });

  it('refuses nothing typed, and an address that is none', () => {
    expect(pairingTarget('  ', 'code', HTTP)).toEqual({ ok: false, problem: NO_ADDRESS });
    expect(pairingTarget('http://', 'code', HTTP)).toEqual({ ok: false, problem: NO_ADDRESS });
  });

  it('refuses an address without a code', () => {
    expect(pairingTarget('gmktec', '  ', HTTP)).toEqual({ ok: false, problem: NO_CODE });
  });

  it('refuses a link that carries a token, with or without a code', () => {
    expect(pairingTarget('http://gmktec:4500/?token=secret', 'code', HTTP)).toEqual({
      ok: false,
      problem: TOKEN_LINK,
    });
  });

  it('refuses a plain address from a page that came over HTTPS, which could not talk to it', () => {
    expect(pairingTarget('http://gmktec:4500/?pair=abc', '', HTTPS)).toEqual({ ok: false, problem: NOT_SECURE });
    expect(pairingTarget('http://gmktec:4500/?pair=abc', '', HTTP).ok).toBe(true);
  });
});
