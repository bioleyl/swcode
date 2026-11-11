import { describe, expect, it } from 'vitest';

import { activityLine } from '../src/lib/activity';
import {
  commandLine,
  commandsMatching,
  commandText,
  menuMatching,
  SESSION_COMMANDS,
  skillLine,
  skillText,
  stepIn,
} from '../src/lib/commands';
import { daemonLine } from '../src/lib/daemon-lines';
import { childPath } from '../src/lib/folders';
import { keepingEnd } from '../src/lib/follow-end';
import { ago, contextText, folderName, percent, tokens } from '../src/lib/format';
import { atTop, fillsWindow, scrollAfterOlder } from '../src/lib/history-window';
import { blocksOf } from '../src/lib/markdown';
import { matchCommands, paletteCommands, paletteTarget } from '../src/lib/palette';
import {
  groupByProject,
  preferredProject,
  removalConsequence,
  sessionsInProject,
} from '../src/lib/project-groups';
import { isSkillOn, skillsOn } from '../src/lib/project-skills';
import { SESSION_STATUS_ICONS } from '../src/lib/session-status';
import { sshHostProblem, sshHostsLine, sshHostsOf } from '../src/lib/ssh-hosts';
import { sessionGrantLabel, toolLabel } from '../src/lib/tool-labels';
import { toolMeta } from '../src/lib/tool-meta';

import type { HostRef, Project } from '../../shared/protocol/project.schema';
import type { SessionSummary } from '../../shared/protocol/session.schema';

describe('blocksOf', () => {
  it('splits paragraphs and keeps inline code and bold apart', () => {
    expect(blocksOf('Run `npm test` now.\n\n**Done**')).toEqual([
      {
        kind: 'paragraph',
        spans: [
          { kind: 'text', text: 'Run ' },
          { kind: 'code', text: 'npm test' },
          { kind: 'text', text: ' now.' },
        ],
      },
      { kind: 'paragraph', spans: [{ kind: 'strong', text: 'Done' }] },
    ]);
  });

  it('reads a code fence without its language line', () => {
    expect(blocksOf('Here:\n```ts\nconst a = 1;\n```\nThat is all.')).toEqual([
      { kind: 'paragraph', spans: [{ kind: 'text', text: 'Here:' }] },
      { kind: 'code', text: 'const a = 1;' },
      { kind: 'paragraph', spans: [{ kind: 'text', text: 'That is all.' }] },
    ]);
  });

  it('shows a fence still open while the answer streams as code', () => {
    expect(blocksOf('```sh\nnpm run')).toEqual([{ kind: 'code', text: 'npm run' }]);
  });
});

describe('format', () => {
  it('writes token counts short', () => {
    expect(tokens(940)).toBe('940');
    expect(tokens(24_100)).toBe('24.1k');
    expect(tokens(64_000)).toBe('64k');
  });

  it('caps a share at the whole', () => {
    expect(percent(24, 64)).toBe(38);
    expect(percent(80, 64)).toBe(100);
    expect(percent(1, 0)).toBe(0);
  });

  it('says how full the context is', () => {
    expect(contextText(24_100, 65_536)).toBe('24.1k / 65.5k');
    expect(contextText(1200, undefined)).toBe('1.2k tokens');
    expect(contextText(0, 65_536)).toBe('');
  });

  it('says how long ago', () => {
    const now = new Date('2026-10-23T12:00:00Z');
    expect(ago('2026-10-23T11:59:40Z', now)).toBe('just now');
    expect(ago('2026-10-23T11:45:00Z', now)).toBe('15 min ago');
    expect(ago('2026-10-23T09:00:00Z', now)).toBe('3 h ago');
  });

  it('names a workspace by its last folder', () => {
    expect(folderName('/home/me/projects/swcode/')).toBe('swcode');
    expect(folderName('C:\\Users\\me\\projects\\swcode')).toBe('swcode');
  });
});

describe('toolMeta', () => {
  it('shows a change to a file by its size, in blue', () => {
    const display = { added: 9, kind: 'diff' as const, lines: [], path: 'a.ts', removed: 2 };
    expect(toolMeta('done', { display, meta: undefined, ok: true })).toEqual({ text: '+9 −2', tone: 'accent' });
  });

  it('shows what the tool said, and failures in red', () => {
    expect(toolMeta('done', { display: undefined, meta: '142 lines', ok: true })).toEqual({
      text: '142 lines',
      tone: 'muted',
    });
    expect(toolMeta('failed', { display: undefined, meta: 'exit code 1', ok: false })).toEqual({
      text: 'exit code 1',
      tone: 'danger',
    });
    expect(toolMeta('waiting', undefined)).toEqual({ text: 'Waiting for you', tone: 'accent' });
  });
});

describe('tool labels', () => {
  it('name each tool and what it asks', () => {
    expect(toolLabel('bash')).toMatchObject({ name: 'Bash', question: 'Run this command?' });
    expect(toolLabel(undefined).name).toBe('Tool');
  });

  it('offer a grant for the session only where one would cover the call', () => {
    expect(sessionGrantLabel('bash', 'npm test && npm run lint')).toBe('Always allow npm in this session');
    expect(sessionGrantLabel('bash', 'echo $(date)')).toBeUndefined();
    expect(sessionGrantLabel('edit', 'src/a.ts')).toBe('Allow all edits in this session');
    expect(sessionGrantLabel('read', 'src/a.ts')).toBeUndefined();
  });
});

describe('activityLine', () => {
  it('says what the agent does in a few words', () => {
    expect(activityLine({ kind: 'working' })).toBe('Working…');
    expect(activityLine({ kind: 'thinking' })).toBe('Thinking…');
    expect(activityLine({ kind: 'writing' })).toBe('Writing…');
  });

  it('names the tool the model calls, then what the call does once it runs', () => {
    expect(activityLine({ kind: 'calling', tool: 'write' })).toBe('Writing a file…');
    expect(activityLine({ kind: 'running', summary: 'src/greet.ts', tool: 'read' })).toBe('Reading src/greet.ts…');
    expect(activityLine({ kind: 'running', summary: 'npm test', tool: 'bash' })).toBe('Running npm test…');
  });

  it('falls back on the tool alone when the call has no summary', () => {
    expect(activityLine({ kind: 'running', summary: '', tool: 'question' })).toBe('Writing questions…');
    expect(activityLine({ kind: 'running', summary: 'x', tool: undefined })).toBe('Calling x…');
  });
});

describe('groupByProject', () => {
  const session = (id: string, workspace: string, host: HostRef = { kind: 'local' }): SessionSummary => ({
    concise: false,
    host,
    id,
    jobs: [],
    kind: 'code',
    mode: 'auto',
    pins: [],
    queued: [],
    queuedNow: false,
    status: 'idle',
    subagents: 0,
    thinking: 'full',
    title: id,
    updatedAt: '2026-10-23T12:00:00.000Z',
    workspace,
  });
  const project = (id: string, path: string, host: HostRef = { kind: 'local' }): Project => ({
    createdAt: '2026-10-23T12:00:00.000Z',
    host,
    id,
    name: id,
    path,
    skillsOff: [],
    todos: [],
  });
  const box: HostRef = { endpoint: 'build-box', kind: 'ssh' };
  const Projects = [project('shop', '/w/shop'), project('api', '/w/api'), project('blog', '/w/blog')];

  it('lists the projects worked in last first, then the others by name', () => {
    const groups = groupByProject(Projects, [
      session('a', '/w/shop'),
      session('b', '/w/api'),
      session('c', '/w/shop'),
    ]);
    expect(groups.map((group) => [group.name, group.sessions.map((each) => each.id)])).toEqual([
      ['shop', ['a', 'c']],
      ['api', ['b']],
      ['blog', []],
    ]);
  });

  it('keeps a session whose folder has no project yet, without a way to add to it', () => {
    expect(groupByProject([], [session('a', '/w/loose')])).toMatchObject([
      { name: 'loose', projectId: undefined, sessions: [{ id: 'a' }] },
    ]);
  });

  it('keeps the same folder on two hosts apart', () => {
    const here = project('here', '/w/app');
    const there = project('there', '/w/app', box);
    const sessions = [session('local', '/w/app'), session('remote', '/w/app', box)];
    expect(sessionsInProject(there, sessions).map((each) => each.id)).toEqual(['remote']);
    expect(sessionsInProject(here, sessions).map((each) => each.id)).toEqual(['local']);
    expect(
      groupByProject([here, there], sessions).map((group) => [
        group.projectId,
        group.sessions.map((each) => each.id),
      ])
    ).toEqual([
      [here.id, ['local']],
      [there.id, ['remote']],
    ]);
  });

  it('starts a new session in the project asked for, else the last one worked in, else the first', () => {
    const sessions = [session('a', '/w/api')];
    expect(preferredProject(Projects, sessions, 'blog')?.id).toBe('blog');
    expect(preferredProject(Projects, sessions, undefined)?.id).toBe('api');
    expect(preferredProject(Projects, [], 'gone')?.id).toBe('shop');
    expect(preferredProject([], [], undefined)).toBeUndefined();
  });

  it('starts it in the project of the host the last session worked on', () => {
    const here = project('here', '/w/app');
    const there = project('there', '/w/app', box);
    expect(preferredProject([here, there], [session('a', '/w/app', box)], undefined)?.id).toBe('there');
    expect(preferredProject([here, there], [session('a', '/w/app')], undefined)?.id).toBe('here');
  });
});

describe('the conversation’s window of events', () => {
  // A window 100 pixels high, scrolled to 60.
  const box = (scrollTop: number, scrollHeight = 500) => ({ clientHeight: 100, scrollHeight, scrollTop });

  describe('atTop', () => {
    it('says yes at the top and just below it', () => {
      expect(atTop(box(0), 120)).toBe(true);
      expect(atTop(box(120), 120)).toBe(true);
    });

    it('says no past the distance', () => {
      expect(atTop(box(121), 120)).toBe(false);
    });
  });

  describe('scrollAfterOlder', () => {
    // Older events were added above: the browser moved the scroll by their height, to 360 of 800.
    const grown = () => box(360, 800);

    it('goes back to the end for people who were there', () => {
      expect(scrollAfterOlder(grown(), true)).toBe(800);
    });

    it('leaves those who scrolled up where the browser put them, below the page that came in', () => {
      expect(scrollAfterOlder(grown(), false)).toBe(360);
    });
  });

  describe('fillsWindow', () => {
    // What a page drew, in pixels: an element's own scroll height never says it, being the window at least.
    it('asks for older events while what is shown is shorter than the window', () => {
      expect(fillsWindow(box(0), 300, 40)).toBe(true);
    });

    it('never asks when the session holds nothing older', () => {
      expect(fillsWindow(box(0), 0, 40)).toBe(false);
    });

    it('never asks once the window is full of events', () => {
      expect(fillsWindow(box(0), 300, 100)).toBe(false);
      expect(fillsWindow(box(0), 300, 500)).toBe(false);
    });
  });
});

describe('keepingEnd', () => {
  // A box 100 pixels high whose content grows by 50 pixels.
  const grow = (scrollTop: number) => {
    const box = { clientHeight: 100, scrollHeight: 300, scrollTop };
    keepingEnd(box, 16, () => {
      box.scrollHeight = 350;
    });
    return box.scrollTop;
  };

  it('keeps the newest content in view while people look at the end', () => {
    expect(grow(200)).toBe(350);
    expect(grow(190)).toBe(350);
  });

  it('leaves people where they scrolled up to', () => {
    expect(grow(120)).toBe(120);
  });
});

describe('removalConsequence', () => {
  it('says what goes with the project, and that its folder stays', () => {
    expect(removalConsequence(0)).toBe('Its folder and files stay.');
    expect(removalConsequence(1)).toBe(
      'Its session goes with it, their conversations for good. Its folder and files stay.'
    );
    expect(removalConsequence(3)).toMatch(/^Its 3 sessions go with it.*Its folder and files stay\.$/);
  });
});

describe('childPath', () => {
  it('joins a folder and a name, the root too', () => {
    expect(childPath('/home/me', 'shop')).toBe('/home/me/shop');
    expect(childPath('/', 'srv')).toBe('/srv');
  });
});

describe('daemonLine', () => {
  const source = (name: string, reachable: boolean) => ({
    id: name,
    models: [],
    name,
    reachable,
    url: `http://${name}:8080`,
  });
  const statusOf = (...sources: ReturnType<typeof source>[]) => ({
    llama: { presets: [], sources },
    workspace: '/w',
  });

  it('tells whether the llm-servers answer, naming the one that does not', () => {
    expect(
      daemonLine({ kind: 'ready', status: statusOf(source('gmktec', true), source('garfield', true)) })
    ).toEqual({
      detail: 'gmktec:8080, garfield:8080',
      text: 'llm-server connected',
      tone: 'ok',
    });
    expect(
      daemonLine({ kind: 'ready', status: statusOf(source('gmktec', true), source('garfield', false)) })
    ).toEqual({
      detail: 'Nothing answers at garfield:8080',
      text: 'garfield unreachable',
      tone: 'problem',
    });
    expect(daemonLine({ kind: 'ready', status: statusOf(source('gmktec', false)) }).text).toBe(
      'llm-server unreachable'
    );
    expect(daemonLine({ kind: 'locked', pairingFailed: false }).tone).toBe('problem');
    expect(daemonLine({ kind: 'locked', pairingFailed: true }).text).toBe('This pairing code expired');
  });
});

describe('the command palette', () => {
  const project: Project = {
    createdAt: '2026-11-02T09:00:00.000Z',
    host: { kind: 'local' },
    id: 'p1',
    name: 'shop',
    path: '/home/me/shop',
    skillsOff: [],
    todos: [],
  };
  const session = (id: string, status: SessionSummary['status']): SessionSummary => ({
    concise: false,
    host: { kind: 'local' },
    id,
    jobs: [],
    kind: 'code',
    mode: 'auto',
    pins: [],
    queued: [],
    queuedNow: false,
    status,
    subagents: 0,
    thinking: 'full',
    title: `Session ${id}`,
    updatedAt: '2026-11-02T10:00:00.000Z',
    workspace: '/home/me/shop',
  });

  it('finds the actions, the sessions that need the person first, the projects and the settings', () => {
    const commands = paletteCommands([project], [session('a', 'idle'), session('b', 'waiting')], ['models']);
    expect(commands.map((command) => command.id)).toEqual([
      'new-session',
      'new-chat',
      'new-project',
      'phone',
      'theme',
      'session:b',
      'session:a',
      'project:p1',
      'settings:models',
    ]);
    expect(commands.find((command) => command.id === 'session:b')?.description).toBe('Needs you · /home/me/shop');
  });

  it('says what a command stands for', () => {
    expect(paletteTarget('session:b')).toEqual({ id: 'b', kind: 'session' });
    expect(paletteTarget('match:b')).toEqual({ id: 'b', kind: 'session' });
    expect(paletteTarget('settings:plugins')).toEqual({ id: 'plugins', kind: 'topic' });
    expect(paletteTarget('phone')).toEqual({ action: 'phone', kind: 'action' });
    expect(paletteTarget('elsewhere')).toBeUndefined();
  });

  it('lists the sessions found in their conversations, where and the words around, leaving out those it does not know', () => {
    const matches = [
      { sessionId: 'b', snippet: '…the prompt cache is lost…' },
      { sessionId: 'gone', snippet: 'cache' },
    ];
    expect(matchCommands(matches, [session('a', 'idle'), session('b', 'working')])).toEqual([
      {
        description: 'shop · …the prompt cache is lost…',
        group: 'In conversations',
        icon: SESSION_STATUS_ICONS.working,
        id: 'match:b',
        label: 'Session b',
        result: true,
      },
    ]);
  });
});

describe('commands', () => {
  const code = SESSION_COMMANDS.code;

  it('read the question of a /peek line, and nothing of a message', () => {
    expect(commandLine('/peek what are you doing?', code)).toEqual({
      argument: 'what are you doing?',
      name: 'peek',
    });
    expect(commandLine('  /peek\nwhy that file?  ', code)).toEqual({ argument: 'why that file?', name: 'peek' });
    expect(commandLine('/peek', code)).toEqual({ argument: '', name: 'peek' });
    expect(commandLine('/peeking at it', code)).toBeUndefined();
    expect(commandLine('please /peek', code)).toBeUndefined();
    expect(commandLine('/todo write the tests', code)).toEqual({ argument: 'write the tests', name: 'todo' });
    expect(commandLine('/todo', code)).toEqual({ argument: '', name: 'todo' });
    expect(commandLine('/todo take out the trash', SESSION_COMMANDS.chat)).toBeUndefined();
  });

  it('run a command that takes no words alone, and leave one that is not available here as words', () => {
    expect(commandLine('/compact', code)).toEqual({ argument: '', name: 'compact' });
    expect(commandLine('/New ', code)).toEqual({ argument: '', name: 'new' });
    expect(commandLine('/compact the tests first', code)).toBeUndefined();
    expect(commandLine('/peek why?', SESSION_COMMANDS.chat)).toBeUndefined();
    expect(commandLine('/etc/hosts is broken', code)).toBeUndefined();
  });

  it('list those starting as typed while a name is typed, in their order', () => {
    expect(commandsMatching('/', code)).toEqual(['new', 'compact', 'todo', 'peek', 'help']);
    expect(commandsMatching('/', SESSION_COMMANDS.chat)).toEqual(['new', 'compact', 'help']);
    expect(commandsMatching('/C', code)).toEqual(['compact']);
    expect(commandsMatching('/peek ', code)).toEqual([]);
    expect(commandsMatching('/x', code)).toEqual([]);
    expect(commandsMatching('hello /', code)).toEqual([]);
    expect(commandsMatching('/', [])).toEqual([]);
  });

  it('list the skills after the commands, one named as a command left to it, as a skill runs as a message', () => {
    const skills = [
      { description: 'Deploys.', name: 'deploy' },
      { description: 'Writes notes.', name: 'Release-Notes' },
      { description: 'Not this one.', name: 'compact' },
    ];
    expect(menuMatching('/', ['compact'], skills)).toEqual([
      { kind: 'command', name: 'compact' },
      { kind: 'skill', skill: skills[0] },
      { kind: 'skill', skill: skills[1] },
    ]);
    expect(menuMatching('/re', code, skills)).toEqual([{ kind: 'skill', skill: skills[1] }]);
    expect(menuMatching('/deploy staging', code, skills)).toEqual([]);
    expect(skillLine('deploy')).toBe('/deploy');
    expect(skillText('deploy')).toBe('/deploy ');
  });

  it('complete a command with a space when words follow it, and step round the list', () => {
    expect(commandText('peek')).toBe('/peek ');
    expect(commandText('compact')).toBe('/compact');
    expect(stepIn(3, 1, 4)).toBe(0);
    expect(stepIn(0, -1, 4)).toBe(3);
    expect(stepIn(0, 1, 0)).toBe(0);
  });
});

describe('a project’s skills', () => {
  const project: Project = {
    createdAt: '2026-11-06T09:00:00.000Z',
    host: { kind: 'local' },
    id: 'p1',
    name: 'shop',
    path: '/w/shop',
    skillsOff: ['deploy'],
    todos: [],
  };
  const found = [
    {
      description: 'Deploys.',
      folder: '/home/me/.agents/skills/deploy',
      name: 'deploy',
      origin: 'personal' as const,
    },
    { description: 'Notes.', folder: '/w/shop/.agents/skills/notes', name: 'notes', origin: 'project' as const },
  ];

  it('are on until the project turns them off, and the composer runs those on', () => {
    expect(isSkillOn(project, 'deploy')).toBe(false);
    expect(isSkillOn(project, 'notes')).toBe(true);
    expect(skillsOn(found, project)).toEqual([{ description: 'Notes.', name: 'notes' }]);
  });
});

describe('SSH hosts', () => {
  it('read the setting’s line by commas or spaces, and write it by commas', () => {
    expect(sshHostsOf('me@box, gmktec  laptop,')).toEqual(['me@box', 'gmktec', 'laptop']);
    expect(sshHostsOf('')).toEqual([]);
    expect(sshHostsLine(['me@box', 'gmktec'])).toBe('me@box, gmktec');
  });

  it('take one host the list does not have', () => {
    expect(sshHostProblem('me@box', ['gmktec'])).toBe('');
    expect(sshHostProblem('', [])).not.toBe('');
    expect(sshHostProblem('me@box gmktec', [])).toBe('One host, without spaces or commas.');
    expect(sshHostProblem('gmktec', ['gmktec'])).toBe('This host is in the list already.');
  });
});
