import { describe, expect, it } from 'vitest';

import { appUpdateNote } from '../src/lib/app-update';

describe('what the Android app’s look says under its button', () => {
  it('names the release Android is installing', () => {
    expect(appUpdateNote({ kind: 'newer', version: '0.14.0' })).toEqual({
      text: 'Android is installing swcode 0.14.0.',
      tone: 'ok',
    });
  });

  it('says the app is the latest', () => {
    expect(appUpdateNote({ kind: 'latest' })).toEqual({ text: 'swcode is the latest version.', tone: 'ok' });
  });

  it('says why it could not look, in the danger tone', () => {
    expect(appUpdateNote({ kind: 'failed', problem: 'HTTP 503 at GitHub' })).toEqual({
      text: 'HTTP 503 at GitHub',
      tone: 'danger',
    });
  });
});
