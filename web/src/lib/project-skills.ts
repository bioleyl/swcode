import type { Project } from '../../../shared/protocol/project.schema';
import type { ProjectSkill, SkillOrigin, SkillSummary } from '../../../shared/protocol/skill.schema';

/** Where a skill was found, as a project's page says it: the person's ~/.agents/skills, or the project's. */
export const SKILL_ORIGIN_LABELS: Readonly<Record<SkillOrigin, string>> = {
  personal: 'Yours',
  project: 'Project',
};

/** Whether a project's sessions have a skill: every one found is on until the project turns it off. */
export const isSkillOn = (project: Project, name: string): boolean => !project.skillsOff.includes(name);

/** The skills a project's sessions have, for the composer: those found, but those the project turned off. */
export const skillsOn = (found: readonly ProjectSkill[], project: Project): SkillSummary[] =>
  found
    .filter((skill) => isSkillOn(project, skill.name))
    .map((skill) => ({ description: skill.description, name: skill.name }));
