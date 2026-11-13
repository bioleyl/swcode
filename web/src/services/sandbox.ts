import { ENDPOINTS, PROJECT_PARAM } from '../../../shared/protocol/endpoints';
import { SandboxViewSchema } from '../../../shared/protocol/project.schema';
import { jsonOf, request } from './api';
import { daemon } from './daemon';

import type { SandboxView } from '../../../shared/protocol/project.schema';

/** Whether a project's host can run its sessions' commands in a sandbox; nothing when the daemon does not answer. */
export const readSandbox = async (projectId: string): Promise<SandboxView | undefined> => {
  const response = await request('GET', `${ENDPOINTS.sandbox}?${PROJECT_PARAM}=${encodeURIComponent(projectId)}`);
  const parsed = SandboxViewSchema.safeParse(await jsonOf(response));
  return parsed.success ? parsed.data : undefined;
};

/** Says whether the project's sessions run their commands in a sandbox: the daemon sends the projects back changed. */
export const setProjectSandbox = (projectId: string, on: boolean): void =>
  daemon.send({ on, projectId, type: 'setProjectSandbox' });
