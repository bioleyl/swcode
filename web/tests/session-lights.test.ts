import { describe, expect, it } from 'vitest';

import { HOST_ICONS } from '../src/lib/hosts';
import { LIT_ICONS, litIcon, projectLight, SESSION_ICON, seenAtOf, sessionLight } from '../src/lib/session-lights';

const AT = '2026-11-05T12:00:00Z';
const LATER = '2026-11-05T12:05:00Z';

describe('sessionLight', () => {
  it('shows a session at work or waiting for the person, seen or not', () => {
    expect(sessionLight({ status: 'working', subagents: 0, updatedAt: AT }, AT)).toBe('working');
    expect(sessionLight({ status: 'waiting', subagents: 0, updatedAt: AT }, AT)).toBe('waiting');
    expect(sessionLight({ status: 'working', subagents: 0, updatedAt: LATER }, undefined)).toBe('working');
  });

  it('shows a finished session done until the person saw it as it is now', () => {
    expect(sessionLight({ status: 'idle', subagents: 0, updatedAt: LATER }, AT)).toBe('done');
    expect(sessionLight({ status: 'idle', subagents: 0, updatedAt: AT }, undefined)).toBe('done');
    expect(sessionLight({ status: 'idle', subagents: 0, updatedAt: AT }, AT)).toBe('seen');
  });
});

describe('seenAtOf', () => {
  it('is what the daemon kept the person saw, on any device', () => {
    expect(seenAtOf({ seenAt: AT, updatedAt: LATER })).toBe(AT);
    expect(
      sessionLight({ status: 'idle', subagents: 0, updatedAt: LATER }, seenAtOf({ seenAt: AT, updatedAt: LATER }))
    ).toBe('done');
  });

  it('counts a session of a daemon that keeps none as seen, not done for good', () => {
    const session = { status: 'idle', subagents: 0, updatedAt: LATER } as const;
    expect(seenAtOf(session)).toBe(LATER);
    expect(sessionLight(session, seenAtOf(session))).toBe('seen');
  });
});

describe('projectLight', () => {
  it('takes the light of the session that most needs the person', () => {
    expect(projectLight(['seen', 'done', 'working', 'waiting'])).toBe('waiting');
    expect(projectLight(['done', 'seen', 'working'])).toBe('working');
    expect(projectLight(['seen', 'done'])).toBe('done');
    expect(projectLight(['seen'])).toBe('seen');
    expect(projectLight([])).toBe('seen');
  });
});

describe('litIcon', () => {
  it('names a lit icon for each light but seen, which keeps the plain icon', () => {
    expect(litIcon(SESSION_ICON, 'working')).toBe('message-working');
    expect(litIcon(SESSION_ICON, 'seen')).toBe(SESSION_ICON);
  });

  it('has every lit icon the navigation names registered, for the bubble and each host', () => {
    for (const icon of [SESSION_ICON, ...Object.values(HOST_ICONS)]) {
      for (const light of ['waiting', 'working', 'done'] as const) {
        expect(Object.hasOwn(LIT_ICONS, litIcon(icon, light))).toBe(true);
      }
    }
  });

  it('fills the shapes with the light’s colour, and keeps the server’s dots in the icon’s ink', () => {
    expect(LIT_ICONS['folder-done']?.[0]?.[1].style).toBe('fill: var(--jl-ok); stroke: var(--jl-ok)');
    expect(LIT_ICONS['server-waiting']?.find(([tag]) => tag === 'line')?.[1].style).toBeUndefined();
  });
});
