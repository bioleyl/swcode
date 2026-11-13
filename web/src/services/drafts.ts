import { z } from 'zod';

import { withDraft } from '../lib/drafts';
import { readStored, writeStored } from './device-store';

import type { PictureUpload } from '../../../shared/protocol/picture.schema';
import type { PromptDraft } from '../types/composer.types';

const STORAGE_KEY = 'swcode.drafts';
const DraftTextsSchema = z.record(z.string(), z.string());

// The pictures waiting with each draft: too large for the storage, they last as long as the page.
const pictures = new Map<string, PictureUpload[]>();

/**
 * The message a composer was left with, for the place it writes for: what was typed there shows again when the
 * person comes back, on this browser, until it is sent or erased.
 */
export function draftOf(place: string): PromptDraft {
  return {
    pictures: pictures.get(place) ?? [],
    text: readStored(STORAGE_KEY, DraftTextsSchema, {})[place] ?? '',
  };
}

/** Keeps what a composer holds now; an empty one forgets the place. */
export function keepDraft(place: string, draft: PromptDraft): void {
  draft.pictures.length === 0 ? pictures.delete(place) : pictures.set(place, draft.pictures);
  writeStored(STORAGE_KEY, withDraft(readStored(STORAGE_KEY, DraftTextsSchema, {}), place, draft.text));
}
