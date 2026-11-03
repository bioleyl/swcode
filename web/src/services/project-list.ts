import { Bus } from '@jadis/core';

import { daemonBus } from './daemon';

import type { Project } from '../../../shared/protocol/project.schema';

// The list lives as long as the page: its bus listener never needs to stop.
const forever = new AbortController().signal;

/** The projects as the daemon last listed them; `open` asks for the dialog that makes one, `remove` to remove one. */
export const projectListBus = new Bus<{ changed: undefined; open: undefined; remove: string }>();

const state = { projects: [] as Project[] };

export const allProjects = (): readonly Project[] => state.projects;

export const findProject = (id: string): Project | undefined =>
  state.projects.find((project) => project.id === id);

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
