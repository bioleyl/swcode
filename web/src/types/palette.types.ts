import type { PALETTE_ACTION } from '../lib/palette';

/** What a command of the palette stands for: one of its actions, a session, a project or a settings topic, or an
 * event a session's conversation says the search words in. */
export type PaletteTarget =
  | { kind: 'action'; action: (typeof PALETTE_ACTION)[keyof typeof PALETTE_ACTION] }
  | { kind: 'session'; id: string }
  | { kind: 'match'; id: string; eventId: string }
  | { kind: 'project'; id: string }
  | { kind: 'topic'; id: string };

export type PaletteTargetMap = { [Target in PaletteTarget as Target['kind']]: Target };
