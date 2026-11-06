import { Bus } from '@jadis/core';

import { ENDPOINTS, PROJECT_PARAM } from '../../../shared/protocol/endpoints';
import { ProjectSkillsSchema } from '../../../shared/protocol/skill.schema';
import { jsonOf, request } from './api';
import { daemon } from './daemon';

import type { ProjectSkill, ProjectSkills } from '../../../shared/protocol/skill.schema';

/** The skills of a project as last read, by its id: `changed` names the project read again. */
export const projectSkillsBus = new Bus<{ changed: string }>();

const found = new Map<string, readonly ProjectSkill[]>();

/** The skills a project's sessions find, as last read; none before they were. */
export const foundSkills = (projectId: string): readonly ProjectSkill[] => found.get(projectId) ?? [];

/**
 * Reads again the skills a project's sessions find, on its host: what the daemon answered, the bus telling once they
 * came; nothing when the daemon does not answer.
 */
export async function readProjectSkills(projectId: string): Promise<ProjectSkills | undefined> {
  const query = new URLSearchParams({ [PROJECT_PARAM]: projectId });
  const parsed = ProjectSkillsSchema.safeParse(await jsonOf(await request('GET', `${ENDPOINTS.skills}?${query}`)));
  if (!parsed.success) {
    return undefined;
  }
  if (parsed.data.problem === undefined) {
    found.set(projectId, parsed.data.skills);
    projectSkillsBus.emit('changed', projectId);
  }
  return parsed.data;
}

/** Turns a skill on or off for a project's sessions: the project comes back changed with the list. */
export const setProjectSkill = (projectId: string, name: string, on: boolean): void =>
  daemon.send({ name, on, projectId, type: 'setProjectSkill' });
