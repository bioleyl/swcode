import { z } from 'zod';

import { readStored, writeStored } from './device-store';

const STORAGE_KEY = 'swcode.nav.folded';

// Without storage, the side nav opens unfolded as long as the page lives.
const memory = { folded: false };

/** Whether the side nav is folded into its rail, as people left it on this device. */
export const navFolded = (): boolean => readStored(STORAGE_KEY, z.boolean(), memory.folded);

/** Folds the side nav into its rail, or unfolds it, and keeps the choice for the next visit. */
export function keepNavFolded(folded: boolean): void {
  memory.folded = folded;
  writeStored(STORAGE_KEY, folded);
}
