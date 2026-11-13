import { describe, expect, it } from 'vitest';

import { projectFolderOf, sessionInProject, sessionsInProject } from '../src/lib/project-groups';
import { sessionPlace } from '../src/lib/session-kinds';

import type { SessionSummary } from '../../shared/protocol/session.schema';

const session = (changes: Partial<SessionSummary>): SessionSummary => ({
  concise: false,
  host: { kind: 'local' },
  id: 's1',
  jobs: [],
  kind: 'code',
  mode: 'auto',
  pins: [],
  preset: '',
  queued: [],
  queuedNow: false,
  status: 'idle',
  subagents: 0,
  thinking: 'full',
  title: 'Fix it',
  updatedAt: '2026-11-13T00:00:00.000Z',
  workspace: '/home/someone/app',
  ...changes,
});

const project = { host: { kind: 'local' as const }, path: '/home/someone/app' };
const inWorktree = session({
  branch: 'swcode/1234abcd',
  id: 's2',
  workspace: '/home/someone/app.swcode-1234abcd',
  worktreeOf: '/home/someone/app',
});

describe('a session in a git worktree of its own', () => {
  it('is a session of the project its worktree was made from', () => {
    expect(projectFolderOf(inWorktree)).toBe('/home/someone/app');
    expect(projectFolderOf(session({}))).toBe('/home/someone/app');
    expect(sessionInProject(project, inWorktree)).toBe(true);
    expect(sessionInProject({ ...project, path: '/home/someone/app.swcode-1234abcd' }, inWorktree)).toBe(false);
    const sessions = [session({}), inWorktree, session({ id: 's3', workspace: '/elsewhere' })];
    expect(sessionsInProject(project, sessions)).toHaveLength(2);
  });

  it('is not one of the same folder on another host', () => {
    const remote = { host: { endpoint: 'build-box', kind: 'ssh' as const }, path: '/home/someone/app' };
    expect(sessionInProject(remote, inWorktree)).toBe(false);
  });

  it('says its branch after its folder', () => {
    expect(sessionPlace(inWorktree)).toContain('/home/someone/app.swcode-1234abcd · swcode/1234abcd');
    expect(sessionPlace(session({}))).not.toContain(' · swcode/');
  });
});
