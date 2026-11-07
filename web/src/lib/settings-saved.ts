import type { ConfirmOptions } from '@syware/jlib';
import type { Restart } from '../../../shared/protocol/settings.schema';

/** What a page says as it saves the settings file, which restarts swcode on it. */
export const SETTINGS_SAVED = {
  byPerson: 'Saved. Restart swcode serve for them to apply.',
  restarted: 'Saved: swcode runs with them now.',
  restarting: 'Saved. swcode restarts on them: a few seconds…',
  saving: 'Saving…',
} as const;

// Run in a terminal, swcode waits for the person to restart it: saving leaves the sessions as they are.
const PERSON_RESTARTS: Restart = 'person';

/** What a save that restarts swcode does to the sessions open now, said before it saves. */
export const RESTART_WARNING =
  'Saving restarts swcode in a few seconds, and the sessions open now can break with it.';

/** The question a page asks before a save that restarts swcode. */
export const RESTART_QUESTION = {
  confirmLabel: 'Save and restart',
  heading: 'Restart swcode?',
  text: RESTART_WARNING,
  tone: 'danger',
} as const satisfies ConfirmOptions;

/** Whether saving restarts swcode under its sessions: unless the person restarts it, or while that is not known. */
export const restartsOnSave = (restart: Restart | undefined): boolean => restart !== PERSON_RESTARTS;
