import { Bus } from '@jadis/core';

import { sameHost } from '../../../shared/hosts/host-key';
import { daemon, daemonBus } from './daemon';

import type { AllowedRule, HostRef, Project } from '../../../shared/protocol/project.schema';

// The list lives as long as the page: its bus listener never needs to stop.
const forever = new AbortController().signal;

/** The projects as the daemon last listed them; `open` asks for the dialog that makes one, `remove` to remove one. */
export const projectListBus = new Bus<{ changed: undefined; open: undefined; remove: string }>();

const state = { projects: [] as Project[] };

export const allProjects = (): readonly Project[] => state.projects;

export const findProject = (id: string): Project | undefined =>
  state.projects.find((project) => project.id === id);

/** The project of a folder on a host, the one a session works in. */
export const projectAt = (path: string, host: HostRef): Project | undefined =>
  state.projects.find((project) => project.path === path && sameHost(project.host, host));

/** Opens the dialog that makes a project, wherever the app is. */
export const askForProject = (): void => projectListBus.emit('open');

/** Asks to remove a project: the frame confirms first. */
export const askToRemoveProject = (id: string): void => projectListBus.emit('remove', id);

daemonBus.register(
  'message',
  (message) => {
    if (message.type !== 'projects') {
      return;
    }
    state.projects = message.projects;
    projectListBus.emit('changed');
  },
  forever
);

/**
 * Keeps the command the project's sessions run after each file their agent writes or edits; none when empty. The
 * daemon sends the projects back changed.
 */
export const setProjectAfterEdit = (projectId: string, command: string): void =>
  daemon.send({ command, projectId, type: 'setProjectAfterEdit' });

/** Takes back what the project's sessions did without asking: they ask again from their next call. */
export const removeProjectAllowed = (projectId: string, rule: AllowedRule, name: string): void =>
  daemon.send({ name, projectId, rule, type: 'removeProjectAllowed' });

/**
 * Says whether the person trusts the project's folder: the notes and skills it brings are given to the agent from
 * its sessions' next turn, or kept from it.
 */
export const setProjectTrust = (projectId: string, trusted: boolean): void =>
  daemon.send({ projectId, trusted, type: 'setProjectTrust' });
