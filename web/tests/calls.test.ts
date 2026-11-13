import { describe, expect, it } from 'vitest';

import { anyStarting, SERVER_STATES, serverLine } from '../src/lib/mcp-view';
import { ALLOWED_RULE_LABELS, allowedRows } from '../src/lib/project-allowed';
import { projectGrantLabel, sessionGrantLabel, toolLabel } from '../src/lib/tool-labels';

import type { Project } from '../../shared/protocol/project.schema';
import type { McpServer } from '../../shared/protocol/settings.schema';

const server = (changes: Partial<McpServer>): McpServer => ({
  name: 'files',
  state: 'ready',
  target: 'npx -y server-files .',
  tools: 3,
  ...changes,
});

describe('an MCP server’s row', () => {
  it('says its command or address, then its tools once it is ready', () => {
    expect(serverLine(server({}))).toBe('npx -y server-files . · 3 tools');
    expect(serverLine(server({ target: 'https://example.com/mcp', tools: 1 }))).toBe(
      'https://example.com/mcp · 1 tool'
    );
    expect(serverLine(server({ state: 'starting', tools: 0 }))).toBe('npx -y server-files .');
  });

  it('says why a server has no tools when it failed', () => {
    expect(serverLine(server({ problem: 'exec: "npx": not found', state: 'failed', tools: 0 }))).toBe(
      'npx -y server-files . · exec: "npx": not found'
    );
    expect(serverLine(server({ state: 'failed', tools: 0 }))).toBe('npx -y server-files . · It did not start.');
  });

  it('has a badge for each state, and is read again while one starts', () => {
    expect(SERVER_STATES.ready).toEqual({ label: 'Ready', tone: 'ok' });
    expect(SERVER_STATES.failed.tone).toBe('danger');
    expect(anyStarting([server({}), server({ state: 'starting' })])).toBe(true);
    expect(anyStarting([server({}), server({ state: 'failed' })])).toBe(false);
  });
});

describe('a call to an MCP server’s tool', () => {
  it('is asked about as one, and allowed for the session by its server', () => {
    expect(toolLabel('mcp').question).toBe('Call this tool of an MCP server?');
    expect(sessionGrantLabel('mcp', 'github · create_issue {"title":"Bug"}')).toBe(
      'Always allow github’s tools in this session'
    );
    expect(sessionGrantLabel('mcp', 'no server here')).toBeUndefined();
  });
});

describe('a command that leaves the sandbox', () => {
  it('is allowed for the session as such, by its programs', () => {
    expect(sessionGrantLabel('bash', 'git push', true)).toBe(
      'Always allow git outside the sandbox in this session'
    );
    expect(sessionGrantLabel('bash', 'git push')).toBe('Always allow git in this session');
    expect(sessionGrantLabel('bash', 'echo $(date)', true)).toBeUndefined();
  });

  it('is named by its command in a terminal, not by the terminal’s title', () => {
    expect(sessionGrantLabel('terminal', 'setup: sudo apt install jq', true)).toBeUndefined();
    expect(sessionGrantLabel('terminal', 'dev server: npm run dev', true)).toBe(
      'Always allow npm outside the sandbox in this session'
    );
    expect(sessionGrantLabel('terminal', 'dev server: npm run dev')).toBe('Always allow npm in this session');
  });
});

describe('an answer for the whole project', () => {
  it('is offered for programs and MCP servers, never for edits', () => {
    expect(projectGrantLabel('bash', 'npm test && git status')).toBe('Always allow npm, git in this project');
    expect(projectGrantLabel('bash', 'git push', true)).toBe(
      'Always allow git outside the sandbox in this project'
    );
    expect(projectGrantLabel('mcp', 'github · create_issue {}')).toBe(
      'Always allow github’s tools in this project'
    );
    expect(projectGrantLabel('edit', 'src/a.ts')).toBeUndefined();
    expect(projectGrantLabel('bash', 'echo $(date)')).toBeUndefined();
    expect(sessionGrantLabel('edit', 'src/a.ts')).toBe('Allow all edits in this session');
  });

  it('is listed in the project’s Config, programs first', () => {
    const project: Project = {
      allowed: { outside: ['git'], programs: ['git', 'npm'], servers: ['github'] },
      createdAt: '2026-11-13T00:00:00.000Z',
      host: { kind: 'local' },
      id: 'p1',
      name: 'app',
      path: '/home/someone/app',
      skillsOff: [],
      todos: [],
    };
    expect(allowedRows(project)).toEqual([
      { name: 'git', rule: 'program' },
      { name: 'npm', rule: 'program' },
      { name: 'git', rule: 'outside' },
      { name: 'github', rule: 'server' },
    ]);
    expect(allowedRows({ ...project, allowed: undefined })).toEqual([]);
    expect(ALLOWED_RULE_LABELS.server.icon).toBe('plug');
  });
});
