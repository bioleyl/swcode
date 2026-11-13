import { z } from 'zod';

import { ComputerColorSchema, THIS_COMPUTER } from '../../../shared/protocol/desktop.schema';
import { readStored, writeStored } from './device-store';

import type { StoredComputer, StoredComputers } from '../types/computers.types';

const STORAGE_KEY = 'swcode.computers';
const NONE: StoredComputers = { computers: [], current: THIS_COMPUTER };

const StoredComputersSchema = z.object({
  computers: z.array(
    z.object({
      address: z.string(),
      color: ComputerColorSchema.optional(),
      id: z.string(),
      name: z.string(),
      token: z.string(),
    })
  ),
  current: z.string(),
});

/**
 * The other computers this browser paired with, and the one it shows: kept in the browser's storage, like the token
 * of the computer that served the page. The desktop app keeps its own, which no page sees.
 */
export const storedComputers = (): StoredComputers => readStored(STORAGE_KEY, StoredComputersSchema, NONE);

export const keepComputers = (kept: StoredComputers): void => writeStored(STORAGE_KEY, kept);

/** The other computer the page talks to; none when it shows the one that served it. */
export function shownComputer(): StoredComputer | undefined {
  const kept = storedComputers();
  return kept.computers.find((computer) => computer.id === kept.current);
}
