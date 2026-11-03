import type { ThinkingMode } from '../../../shared/protocol/session.schema';

/** How the app names each thinking mode: in the composer, and on the settings page. */
export const THINKING_LABELS: Readonly<Record<ThinkingMode, string>> = {
  full: 'Think fully',
  low: 'Think briefly',
  medium: 'Think',
  off: 'Don’t think',
};
