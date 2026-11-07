import { ago } from './format';

import type { Fact } from '@syware/jlib';
import type { Restart } from '../../../shared/protocol/settings.schema';
import type { UpdateView } from '../../../shared/protocol/update.schema';
import type { UpdateNote, UpdateStateMap } from '../types/update.types';

const SOURCE_NOTE =
  'A development build (dev), run from the sources, does not update itself: git pull updates it.';

/** What follows an install: swcode restarts on the new version by itself, or the person restarts it. */
export const INSTALLED_TEXTS: Readonly<Record<Restart, (version: string) => string>> = {
  person: (version) => `swcode ${version} is installed: restart swcode for it to run.`,
  self: (version) => `swcode ${version} is installed: swcode restarts on it, and this page loads again.`,
};

/** The latest release, as each state knows it; none, and the fact is left out. */
const LATEST: { [Kind in keyof UpdateStateMap]: (state: UpdateStateMap[Kind]) => string | null } = {
  current: () => 'This one',
  failed: () => null,
  installed: (state) => `${state.release.version}, installed`,
  newer: (state) => state.release.version,
  source: () => null,
  unchecked: () => 'Not looked for yet',
};

/** What each state says beside the facts, when it has something to say. */
const NOTES: {
  [Kind in keyof UpdateStateMap]: (state: UpdateStateMap[Kind], restart: Restart) => UpdateNote | undefined;
} = {
  current: () => undefined,
  failed: (state) => ({ text: `The last look failed: ${state.problem}`, tone: 'warn' }),
  installed: (state, restart) => ({ text: INSTALLED_TEXTS[restart](state.release.version), tone: 'ok' }),
  newer: () => undefined,
  source: () => ({ text: SOURCE_NOTE, tone: 'info' }),
  unchecked: () => undefined,
};

const latestOf = <Kind extends keyof UpdateStateMap>(kind: Kind, state: UpdateStateMap[Kind]): string | null =>
  LATEST[kind](state);

const noteOf = <Kind extends keyof UpdateStateMap>(
  kind: Kind,
  state: UpdateStateMap[Kind],
  restart: Restart
): UpdateNote | undefined => NOTES[kind](state, restart);

/** The facts the Updates topic lists: the version that runs, the latest release, and when swcode last looked. */
export const updateFacts = (view: UpdateView, now: Date): Fact[] => [
  { code: true, label: 'This version', value: view.version },
  { label: 'Latest release', value: latestOf(view.state.kind, view.state) },
  { label: 'Looked', value: 'at' in view.state ? ago(view.state.at, now) : null },
];

/** What the Updates topic says beside its facts: why it could not look, what to do once installed… */
export const updateNote = (view: UpdateView): UpdateNote | undefined =>
  noteOf(view.state.kind, view.state, view.restart);
