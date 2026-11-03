import type { PermissionMode } from '../../../shared/protocol/session.schema';

/** How the app names each permission mode: what the session asks before it acts. */
export const MODE_LABELS: Readonly<Record<PermissionMode, string>> = {
  acceptEdits: 'Ask before commands',
  ask: 'Ask before edits',
  auto: 'Run everything',
};
