import { daemon } from './daemon';

/** Whether the project's sessions tell the person on the device that shows them: the daemon sends the projects back changed. */
export const setProjectNotifications = (projectId: string, on: boolean): void =>
  daemon.send({ on, projectId, type: 'setProjectNotifications' });
