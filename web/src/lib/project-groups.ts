import { hostKey, sameHost } from '../../../shared/hosts/host-key';
import { folderName } from './format';

import type { HostRef, Project } from '../../../shared/protocol/project.schema';
import type { SessionSummary } from '../../../shared/protocol/session.schema';
import type { ProjectGroup } from '../types/conversation.types';

/** A folder on a host: the same path on two hosts is two folders. */
interface Place {
  host: HostRef;
  path: string;
}

const placeKey = (place: Place): string => `${hostKey(place.host)}|${place.path}`;

const isIn = (session: SessionSummary, place: Place): boolean =>
  session.workspace === place.path && sameHost(session.host, place.host);

/** Whether a session works in a project's folder on the project's host: the same path on two hosts is two projects. */
export const sessionInProject = (project: Pick<Project, 'host' | 'path'>, session: SessionSummary): boolean =>
  isIn(session, project);

/** The sessions working in a project, only those: a session of the same folder on another host stays there. */
export const sessionsInProject = (
  project: Pick<Project, 'host' | 'path'>,
  sessions: readonly SessionSummary[]
): SessionSummary[] => sessions.filter((session) => sessionInProject(project, session));

const groupOf = (project: Project, sessions: readonly SessionSummary[]): ProjectGroup => ({
  host: project.host,
  name: project.name,
  path: project.path,
  projectId: project.id,
  sessions: sessionsInProject(project, sessions),
});

// A session whose folder has no project yet, between two lists from the daemon.
const looseGroup = (place: Place, sessions: readonly SessionSummary[]): ProjectGroup => ({
  host: place.host,
  name: folderName(place.path),
  path: place.path,
  projectId: undefined,
  sessions: sessions.filter((session) => isIn(session, place)),
});

/**
 * The projects with their sessions, for the side list: the ones worked in last first (the sessions come newest
 * first), then the ones without sessions, by name.
 */
export function groupByProject(projects: readonly Project[], sessions: readonly SessionSummary[]): ProjectGroup[] {
  const places = new Map(
    sessions.map((session) => [placeKey({ host: session.host, path: session.workspace }), session])
  );
  const worked = [...places.values()].map((session) => ({ host: session.host, path: session.workspace }));
  const withSessions = worked.map((place) => {
    const project = projects.find((known) => placeKey(known) === placeKey(place));
    return project ? groupOf(project, sessions) : looseGroup(place, sessions);
  });
  const workedKeys = new Set(places.keys());
  const idle = projects
    .filter((project) => !workedKeys.has(placeKey(project)))
    .sort((first, second) => first.name.localeCompare(second.name))
    .map((project) => groupOf(project, sessions));
  return [...withSessions, ...idle];
}

/** What removing a project takes with it, said in the dialog that asks first: its sessions, not its folder. */
export function removalConsequence(sessions: number): string {
  const kept = 'Its folder and files stay.';
  if (sessions === 0) {
    return kept;
  }
  const what = sessions === 1 ? 'Its session goes' : `Its ${sessions} sessions go`;
  return `${what} with it, their conversations for good. ${kept}`;
}

/** The project a new session goes in: the one asked for, else the one worked in last, else the first. */
export function preferredProject(
  projects: readonly Project[],
  sessions: readonly SessionSummary[],
  askedId: string | undefined
): Project | undefined {
  const asked = projects.find((project) => project.id === askedId);
  const latest = sessions[0];
  const last = latest === undefined ? undefined : projects.find((project) => sessionInProject(project, latest));
  return asked ?? last ?? projects[0];
}
