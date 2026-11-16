import type { Tone } from '@syware/jlib';
import type { AppUpdate, AppUpdateMap } from '../types/app-update.types';
import type { UpdateNote } from '../types/update.types';

/** What each look of the Android app leaves under its button, in an alert of that tone. */
const TEXTS: { [Kind in keyof AppUpdateMap]: (update: AppUpdateMap[Kind]) => string } = {
  failed: (update) => update.problem,
  latest: () => 'swcode is the latest version.',
  newer: (update) => `Android is installing swcode ${update.version}.`,
};

const TONES: { [Kind in keyof AppUpdateMap]: Tone } = {
  failed: 'danger',
  latest: 'ok',
  newer: 'ok',
};

const noteOf = <Kind extends keyof AppUpdateMap>(kind: Kind, update: AppUpdateMap[Kind]): UpdateNote => ({
  text: TEXTS[kind](update),
  tone: TONES[kind],
});

/** What the Android app's own look says under its button, and how it says it. */
export const appUpdateNote = (update: AppUpdate): UpdateNote => noteOf(update.kind, update);
