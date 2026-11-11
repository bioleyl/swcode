import { HOST_ICONS, placeText } from './hosts';
import { sessionPlace, sessionPlaceName } from './session-kinds';
import { liveStatus, SESSION_STATUS_ICONS, SESSION_STATUS_LOOKS } from './session-status';
import { SETTINGS_TOPIC_LABELS } from './settings-topics';

import type { PaletteCommand } from '@syware/jlib';
import type { SessionMatch } from '../../../shared/protocol/messages.schema';
import type { Project } from '../../../shared/protocol/project.schema';
import type { SessionSummary } from '../../../shared/protocol/session.schema';
import type { PaletteTarget } from '../types/palette.types';
import type { SettingsTopic } from './settings-topics';

/** The palette's own actions, by id. */
export const PALETTE_ACTION = {
  newChat: 'new-chat',
  newProject: 'new-project',
  newSession: 'new-session',
  phone: 'phone',
  theme: 'theme',
} as const;
type PaletteAction = (typeof PALETTE_ACTION)[keyof typeof PALETTE_ACTION];

const SESSION_PREFIX = 'session:';
// A session found in its conversation: the same session may be listed by its title too. Its id names the session and
// the event the words are in.
const MATCH_PREFIX = 'match:';
const MATCH_SEPARATOR = '|';
const PROJECT_PREFIX = 'project:';
const TOPIC_PREFIX = 'settings:';
const GROUP = {
  actions: 'Actions',
  conversations: 'In conversations',
  projects: 'Projects',
  sessions: 'Sessions',
  settings: 'Settings',
} as const;

const ACTIONS: readonly PaletteCommand[] = [
  {
    group: GROUP.actions,
    icon: 'plus',
    id: PALETTE_ACTION.newSession,
    keywords: ['start', 'prompt'],
    label: 'New session',
  },
  {
    group: GROUP.actions,
    icon: 'message',
    id: PALETTE_ACTION.newChat,
    keywords: ['ask', 'question', 'web'],
    label: 'New chat',
  },
  {
    group: GROUP.actions,
    icon: 'folder-plus',
    id: PALETTE_ACTION.newProject,
    keywords: ['folder'],
    label: 'New project',
  },
  {
    group: GROUP.actions,
    icon: 'smartphone',
    id: PALETTE_ACTION.phone,
    keywords: ['pair', 'qr'],
    label: 'Use swcode on a phone',
  },
  {
    group: GROUP.actions,
    icon: 'moon',
    id: PALETTE_ACTION.theme,
    keywords: ['dark', 'light'],
    label: 'Switch the theme',
  },
];

const isAction = (id: string): id is PaletteAction => Object.values<string>(PALETTE_ACTION).includes(id);

const sessionCommand = (session: SessionSummary): PaletteCommand => ({
  description: `${SESSION_STATUS_LOOKS[liveStatus(session)].label} · ${sessionPlace(session)}`,
  group: GROUP.sessions,
  icon: SESSION_STATUS_ICONS[liveStatus(session)],
  id: `${SESSION_PREFIX}${session.id}`,
  label: session.title,
});

/** The id of the command that opens a session on the event its conversation says the search words in. */
const matchId = (sessionId: string, eventId: string): string =>
  `${MATCH_PREFIX}${encodeURIComponent(sessionId)}${MATCH_SEPARATOR}${encodeURIComponent(eventId)}`;

const matchCommand = (session: SessionSummary, match: SessionMatch): PaletteCommand => ({
  description: `${sessionPlaceName(session)} · ${match.snippet}`,
  group: GROUP.conversations,
  icon: SESSION_STATUS_ICONS[liveStatus(session)],
  id: matchId(session.id, match.eventId),
  label: session.title,
  result: true,
});

const projectCommand = (project: Project): PaletteCommand => ({
  description: placeText(project.host, project.path),
  group: GROUP.projects,
  icon: HOST_ICONS[project.host.kind],
  id: `${PROJECT_PREFIX}${project.id}`,
  label: project.name,
});

const topicCommand = (topic: SettingsTopic): PaletteCommand => ({
  group: GROUP.settings,
  icon: SETTINGS_TOPIC_LABELS[topic].icon,
  id: `${TOPIC_PREFIX}${topic}`,
  keywords: ['settings', 'preferences'],
  label: SETTINGS_TOPIC_LABELS[topic].label,
});

/** Everything the palette finds: what it can do, then the sessions, those that need the person first, the projects, the settings. */
export function paletteCommands(
  projects: readonly Project[],
  sessions: readonly SessionSummary[],
  topics: readonly SettingsTopic[]
): PaletteCommand[] {
  const waitingFirst = [...sessions].sort(
    (first, second) => Number(second.status === 'waiting') - Number(first.status === 'waiting')
  );
  return [
    ...ACTIONS,
    ...waitingFirst.map(sessionCommand),
    ...projects.map(projectCommand),
    ...topics.map(topicCommand),
  ];
}

/** The sessions the daemon found saying what was typed, each with the words around it; none it does not list. */
export function matchCommands(
  matches: readonly SessionMatch[],
  sessions: readonly SessionSummary[]
): PaletteCommand[] {
  return matches.flatMap((match) => {
    const session = sessions.find((each) => each.id === match.sessionId);
    return session ? [matchCommand(session, match)] : [];
  });
}

/** The session a found conversation is in, and the event the palette found it in. */
const matchTarget = (rest: string): PaletteTarget | undefined => {
  const [sessionId, eventId] = rest.split(MATCH_SEPARATOR).map(decodeURIComponent);
  return sessionId === undefined || eventId === undefined ? undefined : { eventId, id: sessionId, kind: 'match' };
};

/** What a command the palette gave stands for; nothing for an id it never made. */
export function paletteTarget(id: string): PaletteTarget | undefined {
  if (isAction(id)) {
    return { action: id, kind: 'action' };
  }
  if (id.startsWith(MATCH_PREFIX)) {
    return matchTarget(id.slice(MATCH_PREFIX.length));
  }
  const prefixed = (
    [
      [SESSION_PREFIX, 'session'],
      [PROJECT_PREFIX, 'project'],
      [TOPIC_PREFIX, 'topic'],
    ] as const
  ).find(([prefix]) => id.startsWith(prefix));
  return prefixed ? { id: id.slice(prefixed[0].length), kind: prefixed[1] } : undefined;
}
