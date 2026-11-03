import { z } from 'zod';

import { LESSON_ORDER } from '../lib/vim-lessons';
import { readStored, writeStored } from './device-store';

import type { LessonId } from '../types/vim.types';

const STORAGE_KEY = 'swcode.vimGame';

const LessonIdSchema = z.enum(LESSON_ORDER);
const VimProgressSchema = z.object({
  best: z.partialRecord(LessonIdSchema, z.number()),
  lesson: LessonIdSchema,
});
type VimProgress = z.infer<typeof VimProgressSchema>;

const FIRST: VimProgress = { best: {}, lesson: 'steps' };

// Without storage, the progress lasts as long as the page.
const memory = { progress: readStored(STORAGE_KEY, VimProgressSchema, FIRST) };

const keep = (progress: VimProgress): void => {
  memory.progress = progress;
  writeStored(STORAGE_KEY, progress);
};

/** The lesson people took last. */
export const lastLesson = (): LessonId => memory.progress.lesson;

/** The best round of a lesson on this device, in percent of par; 0 before any. */
export const bestOf = (lesson: LessonId): number => memory.progress.best[lesson] ?? 0;

export const keepLesson = (lesson: LessonId): void => keep({ ...memory.progress, lesson });

/** Keeps a round's result when it beats the lesson's best, and gives the best back. */
export function keepBest(lesson: LessonId, percent: number): number {
  const best = Math.max(percent, bestOf(lesson));
  keep({ ...memory.progress, best: { ...memory.progress.best, [lesson]: best } });
  return best;
}
