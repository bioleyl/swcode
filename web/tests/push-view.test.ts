import { describe, expect, it } from 'vitest';

import { isAppleMobile, keyBytes, PUSH_BUTTONS, PUSH_LINES } from '../src/lib/push-view';

const IPHONE =
  'Mozilla/5.0 (iPhone; CPU iPhone OS 18_0 like Mac OS X) AppleWebKit/605.1.15 Version/18.0 Mobile/15E148';
const MAC = 'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 Version/18.0 Safari/605.1.15';
const ANDROID =
  'Mozilla/5.0 (Linux; Android 15; Pixel 9) AppleWebKit/537.36 Chrome/130.0.0.0 Mobile Safari/537.36';

describe('notifications on this device', () => {
  it('offer a button only where a press can change them', () => {
    expect(PUSH_BUTTONS.off).toBe('Notify me on this device');
    expect(PUSH_BUTTONS.on).toBe('Stop notifying this device');
    expect([
      PUSH_BUTTONS.denied,
      PUSH_BUTTONS.insecure,
      PUSH_BUTTONS.needsInstall,
      PUSH_BUTTONS.unsupported,
    ]).toEqual([undefined, undefined, undefined, undefined]);
  });

  it('say why they cannot be turned on, each in its own words', () => {
    expect(PUSH_LINES.insecure).toContain('HTTPS');
    expect(PUSH_LINES.needsInstall).toContain('Home Screen');
    expect(PUSH_LINES.denied).toContain('blocked');
    expect(new Set(Object.values(PUSH_LINES)).size).toBe(Object.keys(PUSH_LINES).length);
  });

  it('know an iPhone and an iPad, which says it is a Mac but has a touch screen', () => {
    expect(isAppleMobile(IPHONE, 5)).toBe(true);
    expect(isAppleMobile(MAC, 5)).toBe(true);
    expect(isAppleMobile(MAC, 0)).toBe(false);
    expect(isAppleMobile(ANDROID, 5)).toBe(false);
  });

  it('read the daemon’s key from base64url, whatever its length', () => {
    expect([...keyBytes('BP-_')]).toEqual([4, 255, 191]);
    expect([...keyBytes('AQI')]).toEqual([1, 2]);
    expect([...keyBytes('AQ')]).toEqual([1]);
    expect(keyBytes('').length).toBe(0);
  });
});
