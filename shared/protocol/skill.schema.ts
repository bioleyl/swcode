import { z } from 'zod';

/** What the agent is told of a skill: its name, and what it is for, which the agent picks it by. */
export const SkillSummarySchema = z.object({ description: z.string(), name: z.string() });
export type SkillSummary = z.infer<typeof SkillSummarySchema>;

/** Where a skill is: the person's `~/.agents/skills`, or the `.agents/skills` of the project or of a folder above it. */
export const SKILL_ORIGINS = ['personal', 'project'] as const;
export const SkillOriginSchema = z.enum(SKILL_ORIGINS);
export type SkillOrigin = z.infer<typeof SkillOriginSchema>;

/** A skill as a project's page lists it: whether the project turned it off is the project's `skillsOff`. */
export const ProjectSkillSchema = z.object({
  description: z.string(),
  // Its folder, on the project's host.
  folder: z.string(),
  name: z.string(),
  origin: SkillOriginSchema,
});
export type ProjectSkill = z.infer<typeof ProjectSkillSchema>;

/**
 * `GET /api/skills?project=<id>`: the skills the project's sessions find, each name once (the person's first), and
 * why its host could not be read, when it could not.
 */
export const ProjectSkillsSchema = z.object({
  problem: z.string().optional(),
  skills: z.array(ProjectSkillSchema),
});
export type ProjectSkills = z.infer<typeof ProjectSkillsSchema>;
