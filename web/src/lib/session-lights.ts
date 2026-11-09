import { Folder, MessageSquare, Server } from 'lucide';

import { HOST_ICONS } from './hosts';
import { liveStatus } from './session-status';

import type { IconNode } from 'lucide';
import type { HostRef } from '../../../shared/protocol/project.schema';
import type { SessionStatus, SessionSummary } from '../../../shared/protocol/session.schema';

/**
 * How a session shows in the navigation, what needs the person first: waiting for them, at work, done since they last
 * saw it, or seen. A project shows the first of its sessions' lights.
 */
export const SESSION_LIGHTS = ['waiting', 'working', 'done', 'seen'] as const;
export type SessionLight = (typeof SESSION_LIGHTS)[number];

/** A session's icon: a speech bubble, filled with its light's colour. */
export const SESSION_ICON = 'message';

// Each light's colour, filling the icon; a session seen since its turn ended keeps the icon empty.
const LIGHT_TONES: Readonly<Record<SessionLight, string | undefined>> = {
  done: 'var(--jl-ok)',
  seen: undefined,
  waiting: 'var(--jl-warn)',
  working: 'var(--jl-accent)',
};

// Each host's project icon, by its name and its shape, which takes its sessions' light.
const HOST_SHAPES: Readonly<Record<HostRef['kind'], [string, IconNode]>> = {
  local: [HOST_ICONS.local, Folder],
  ssh: [HOST_ICONS.ssh, Server],
  wsl: [HOST_ICONS.wsl, Folder],
};

// One shape per icon, since two hosts show the same one: a WSL project's light fills the folder as a local one's.
const PROJECT_SHAPES: ReadonlyMap<string, IconNode> = new Map(
  Object.values(HOST_SHAPES).map(([icon, node]) => [icon, node])
);

// A line keeps the icon's ink: the dots of the server's drawers stay seen over their fill.
const UNFILLED_TAGS: ReadonlySet<string> = new Set(['line']);

const LIGHTS: Readonly<Record<SessionStatus, (seen: boolean) => SessionLight>> = {
  idle: (seen) => (seen ? 'seen' : 'done'),
  waiting: () => 'waiting',
  working: () => 'working',
};

const tinted = (node: IconNode, tone: string): IconNode =>
  node.map(([tag, attrs]) => [
    tag,
    UNFILLED_TAGS.has(tag) ? attrs : { ...attrs, style: `fill: ${tone}; stroke: ${tone}` },
  ]);

/** The name of an icon lit with a light: the plain icon for a session seen. */
export const litIcon = (icon: string, light: SessionLight): string =>
  LIGHT_TONES[light] === undefined ? icon : `${icon}-${light}`;

const litVariants = (icon: string, node: IconNode): [string, IconNode][] =>
  SESSION_LIGHTS.flatMap((light) => {
    const tone = LIGHT_TONES[light];
    return tone === undefined ? [] : [[litIcon(icon, light), tinted(node, tone)]];
  });

/** The lit icons to register, by name: the session's bubble and each host's project icon, filled with each light. */
export const LIT_ICONS: Readonly<Record<string, IconNode>> = Object.fromEntries([
  ...litVariants(SESSION_ICON, MessageSquare),
  ...[...PROJECT_SHAPES].flatMap(([icon, node]) => litVariants(icon, node)),
]);

/**
 * The last activity the person saw of a session, on any device. A daemon from before it kept this says nothing: its
 * sessions count as seen, rather than done for good.
 */
export const seenAtOf = (session: Pick<SessionSummary, 'seenAt' | 'updatedAt'>): string =>
  session.seenAt ?? session.updatedAt;

/** A session's light: done once its turn ended, until the person saw it as it is now (its last activity, `seenAt`). */
export const sessionLight = (
  session: Pick<SessionSummary, 'status' | 'subagents' | 'updatedAt'>,
  seenAt: string | undefined
): SessionLight => LIGHTS[liveStatus(session)](seenAt === session.updatedAt);

/** A project's light: the first of its sessions' in `SESSION_LIGHTS`' order, seen when it has none. */
export const projectLight = (lights: readonly SessionLight[]): SessionLight =>
  SESSION_LIGHTS.find((light) => lights.includes(light)) ?? 'seen';
