import { z } from 'zod';

import { readStored, writeStored } from './device-store';

const STORAGE_KEY = 'swcode.gameBest';

// Without storage, the best score lasts as long as the page.
const memory = { best: 0 };

/** The best score of the space shooter on this device. */
export const bestScore = (): number => Math.max(memory.best, readStored(STORAGE_KEY, z.number(), 0));

/** Keeps a game's score when it beats the best, and gives the best back. */
export function keepScore(score: number): number {
  const best = Math.max(score, bestScore());
  memory.best = best;
  writeStored(STORAGE_KEY, best);
  return best;
}
