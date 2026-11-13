import type { DraftTexts } from '../types/drafts.types';

/** The places a message is written for before its session exists: the page that starts one of each kind. */
export const NEW_DRAFT_PLACES = { chat: 'new-chat', code: 'new-session' } as const;

// The drafts kept at most: the oldest goes when one more comes, so the sessions long gone never fill the storage.
export const MAX_DRAFTS = 50;

/**
 * The drafts once a place's text changed, the latest written last: an empty text forgets the place, and past
 * `MAX_DRAFTS` the ones written longest ago go.
 */
export function withDraft(drafts: DraftTexts, place: string, text: string): DraftTexts {
  const { [place]: _replaced, ...others } = drafts;
  const next = text === '' ? others : { ...others, [place]: text };
  return Object.fromEntries(Object.entries(next).slice(-MAX_DRAFTS));
}
