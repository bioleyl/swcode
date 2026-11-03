import { z } from 'zod';

/** A skill's name, which is its folder's: lowercase letters, digits and hyphens, as Agent Skills have them. */
export const SKILL_NAME_PATTERN = /^[a-z0-9]+(?:-[a-z0-9]+)*$/;
export const MAX_SKILL_NAME = 64;
export const MAX_SKILL_DESCRIPTION = 1024;

export const SkillNameSchema = z
  .string()
  .trim()
  .min(1)
  .max(MAX_SKILL_NAME)
  .regex(SKILL_NAME_PATTERN, 'lowercase letters, digits and hyphens, such as deploy-staging');

/** What the agent is told of a skill: its name, and what it is for, which the agent picks it by. */
export const SkillSummarySchema = z.object({ description: z.string(), name: z.string() });
export type SkillSummary = z.infer<typeof SkillSummarySchema>;

/** A skill as the page and the agent write it: its name, what it is for, and the instructions. */
export const SkillSchema = z.object({
  description: z.string().trim().min(1).max(MAX_SKILL_DESCRIPTION),
  instructions: z.string().trim().min(1),
  name: SkillNameSchema,
});
export type Skill = z.infer<typeof SkillSchema>;

/** `GET /api/skills/<name>`: a skill as its folder has it, with the other files there (references, scripts). */
export const SkillViewSchema = z.object({
  description: z.string(),
  files: z.array(z.string()),
  instructions: z.string(),
  name: z.string(),
});
export type SkillView = z.infer<typeof SkillViewSchema>;

/** `GET /api/skills`: the skills, and the folder they are kept in. */
export const SkillsViewSchema = z.object({ folder: z.string(), skills: z.array(SkillSummarySchema) });
export type SkillsView = z.infer<typeof SkillsViewSchema>;

/** What creating, changing or deleting a skill answers: done, under that name; or why not. */
export const SkillChangeSchema = z.discriminatedUnion('ok', [
  z.object({ name: z.string(), ok: z.literal(true) }),
  z.object({ ok: z.literal(false), problem: z.string() }),
]);
export type SkillChange = z.infer<typeof SkillChangeSchema>;
