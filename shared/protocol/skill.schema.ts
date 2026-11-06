import { z } from 'zod';

/** What the agent is told of a skill: its name, and what it is for, which the agent picks it by. */
export const SkillSummarySchema = z.object({ description: z.string(), name: z.string() });
export type SkillSummary = z.infer<typeof SkillSummarySchema>;
