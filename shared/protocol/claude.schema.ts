import { z } from 'zod';

import { HostOptionSchema } from './project.schema.js';

/**
 * Which of Claude Code's skills swcode reads besides its own: the person's (`~/.claude/skills`, `~/.claude/commands`),
 * the project's (`.claude/skills`, `.claude/commands`), and Claude Code's plugins, each on or off here.
 */
export const ClaudeSourcesSchema = z.object({
  personal: z.boolean().default(true),
  // By Claude Code's id, `name@marketplace`: a plugin left out follows Claude Code, on when it is enabled there.
  plugins: z.record(z.string(), z.boolean()).default({}),
  project: z.boolean().default(true),
});
export type ClaudeSources = z.infer<typeof ClaudeSourcesSchema>;

/** A skill or a command of Claude Code, as the page lists it. */
export const ClaudeSkillViewSchema = z.object({
  description: z.string(),
  // `disable-model-invocation`: Claude Code's person types it as a command; the agent is not offered it.
  forPeople: z.boolean(),
  name: z.string(),
});
export type ClaudeSkillView = z.infer<typeof ClaudeSkillViewSchema>;

/** A plugin Claude Code installed, with whether Claude Code has it enabled and whether swcode reads it. */
export const ClaudePluginViewSchema = z.object({
  description: z.string(),
  enabledInClaude: z.boolean(),
  id: z.string(),
  name: z.string(),
  on: z.boolean(),
  skills: z.array(ClaudeSkillViewSchema),
  version: z.string(),
});
export type ClaudePluginView = z.infer<typeof ClaudePluginViewSchema>;

/** `GET /api/claude?host=<key>`: what swcode finds of Claude Code on a machine, and which of it it reads. */
export const ClaudeViewSchema = z.object({
  // Claude Code's folder there, `~/.claude`.
  folder: z.string(),
  // The machines Claude Code may be on: where projects can be.
  hosts: z.array(HostOptionSchema),
  personal: z.array(ClaudeSkillViewSchema),
  plugins: z.array(ClaudePluginViewSchema),
  // Why that machine could not be read, when it could not.
  problem: z.string().optional(),
  sources: ClaudeSourcesSchema,
});
export type ClaudeView = z.infer<typeof ClaudeViewSchema>;
