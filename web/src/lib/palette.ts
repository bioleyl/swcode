import { HOST_ICONS, placeText } from './hosts';
import { SESSION_STATUS_ICONS, SESSION_STATUS_LOOKS } from './session-status';
import { SETTINGS_TOPIC_LABELS } from './settings-topics';

import type { PaletteCommand } from '@syware/jlib';
import type { Project } from '../../../shared/protocol/project.schema';
import type { SessionSummary } from '../../../shared/protocol/session.schema';
import type { PaletteTarget } from '../types/palette.types';
import type { SettingsTopic } from './settings-topics';

/** The palette's own actions, by id. */
export const PALETTE_ACTION = {
  newProject: 'new-project',
  newSession: 'new-session',
  phone: 'phone',
  theme: 'theme',
} as const;
type PaletteAction = (typeof PALETTE_ACTION)[keyof typeof PALETTE_ACTION];

const SESSION_PREFIX = 'session:';
const PROJECT_PREFIX = 'project:';
const TOPIC_PREFIX = 'settings:';
const GROUP = { actions: 'Actions', projects: 'Projects', sessions: 'Sessions', settings: 'Settings' } as const;

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
  description: `${SESSION_STATUS_LOOKS[session.status].label} · ${placeText(session.host, session.workspace)}`,
  group: GROUP.sessions,
  icon: SESSION_STATUS_ICONS[session.status],
  id: `${SESSION_PREFIX}${session.id}`,
  label: session.title,
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

/** What a command the palette gave stands for; nothing for an id it never made. */
export function paletteTarget(id: string): PaletteTarget | undefined {
  if (isAction(id)) {
    return { action: id, kind: 'action' };
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
