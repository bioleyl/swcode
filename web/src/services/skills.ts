import { ENDPOINTS } from '../../../shared/protocol/endpoints';
import { SkillChangeSchema, SkillsViewSchema, SkillViewSchema } from '../../../shared/protocol/skill.schema';
import { jsonOf, request } from './api';

import type { Skill, SkillChange, SkillsView, SkillView } from '../../../shared/protocol/skill.schema';

const UNREACHABLE = 'The daemon did not answer.';

const skillPath = (name: string): string => `${ENDPOINTS.skills}/${encodeURIComponent(name)}`;

const changeOf = async (response: Response | undefined): Promise<SkillChange> => {
  const parsed = SkillChangeSchema.safeParse(await jsonOf(response));
  return parsed.success ? parsed.data : { ok: false, problem: UNREACHABLE };
};

/** The skills, and the folder the daemon keeps them in. */
export async function listSkills(): Promise<SkillsView | undefined> {
  const parsed = SkillsViewSchema.safeParse(await jsonOf(await request('GET', ENDPOINTS.skills)));
  return parsed.success ? parsed.data : undefined;
}

/** A skill as its folder has it; nothing when there is no such skill. */
export async function loadSkill(name: string): Promise<SkillView | undefined> {
  const parsed = SkillViewSchema.safeParse(await jsonOf(await request('GET', skillPath(name))));
  return parsed.success ? parsed.data : undefined;
}

export const createSkill = async (skill: Skill): Promise<SkillChange> =>
  changeOf(await request('POST', ENDPOINTS.skills, skill));

/** Saves a skill; a new name renames it. */
export const updateSkill = async (name: string, skill: Skill): Promise<SkillChange> =>
  changeOf(await request('PUT', skillPath(name), skill));

export const removeSkill = async (name: string): Promise<SkillChange> =>
  changeOf(await request('DELETE', skillPath(name)));
