import type { SessionDefaults, ThinkingMode } from '../../../shared/protocol/choices.schema';

/** A new session, a chat or a coding one, starts in token economy: the person turns it off in the composer's choices. */
export const CONCISE_AT_FIRST = true;
/** A new coding session runs everything, unless its project or the person says otherwise. */
export const MODE_AT_FIRST = 'auto';
/** The thinking a new session gets when the daemon does not say its own. */
const THINKING_AT_FIRST: ThinkingMode = 'full';

/** What a new session starts with by the app's own defaults: the default preset, and the daemon's thinking. */
export const appDefaults = (thinking: ThinkingMode | undefined): SessionDefaults => ({
  concise: CONCISE_AT_FIRST,
  mode: MODE_AT_FIRST,
  preset: '',
  thinking: thinking ?? THINKING_AT_FIRST,
});

/** What a new session starts with: what the person picked for it, else its project's defaults, else the app's. */
export const startingChoices = (
  app: SessionDefaults,
  project: SessionDefaults | undefined,
  picks: Partial<SessionDefaults>
): SessionDefaults => ({ ...app, ...project, ...picks });
