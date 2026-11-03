import { z } from 'zod';

import { FIRST_PROGRESS, LETTER_ORDER } from '../lib/keyboard-letters';
import { readStored, writeStored } from './device-store';

import type { KeyboardProgress } from '../types/keyboard.types';

const STORAGE_KEY = 'swcode.keyboardGame';

const KeyboardProgressSchema = z.object({
  letters: z.partialRecord(z.string(), z.object({ averageMs: z.number().positive(), samples: z.number().int() })),
  targetWpm: z.number().positive(),
  unlocked: z.number().int().min(FIRST_PROGRESS.unlocked).max(LETTER_ORDER.length),
});

// Without storage, the progress lasts as long as the page.
const memory = { progress: readStored<KeyboardProgress>(STORAGE_KEY, KeyboardProgressSchema, FIRST_PROGRESS) };

/** Where people are on this device: their letters, their target, and how fast they type each letter. */
export const storedProgress = (): KeyboardProgress => memory.progress;

export function keepProgress(progress: KeyboardProgress): void {
  memory.progress = progress;
  writeStored(STORAGE_KEY, progress);
}
