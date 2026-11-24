import { describe, expect, it } from 'vitest';

import { hostConfig } from '../src/services/host';

describe('the host config of the bundle a host ships', () => {
  it('takes nothing from an address with no query', () => {
    expect(hostConfig('')).toEqual({ daemon: '', token: '' });
  });

  it("takes the token the daemon's link brings", () => {
    expect(hostConfig('?token=9f2c')).toEqual({ daemon: '', token: '9f2c' });
  });

  it('takes the daemon a host names, with its port', () => {
    expect(hostConfig('?daemon=http%3A%2F%2F127.0.0.1%3A4500&token=9f2c')).toEqual({
      daemon: 'http://127.0.0.1:4500',
      token: '9f2c',
    });
  });

  it('takes the theme the host names', () => {
    expect(hostConfig('?theme=dark&token=9f2c')).toEqual({ daemon: '', theme: 'dark', token: '9f2c' });
    expect(hostConfig('?theme=light')).toEqual({ daemon: '', theme: 'light', token: '' });
  });

  it('leaves the theme out when the host names one it does not know', () => {
    expect(hostConfig('?theme=solarized')).toEqual({ daemon: '', token: '' });
  });

  it('keeps a value with an equals sign in it', () => {
    expect(hostConfig('?token=ab%3Dcd')).toEqual({ daemon: '', token: 'ab=cd' });
  });
});
