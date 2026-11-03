import type { UseEventsHandler } from '@jadis/core';
import type { GameNotice } from './game.types';

/** The games of the arcade: a space shooter, vim's motions, and touch typing. */
export type GameId = 'drift' | 'motions' | 'keyboard';

/** What every game of the arcade does: runs while it shows, and pauses when hidden or when the session wants the person. */
export interface ArcadeGame extends HTMLElement {
  readonly events: Readonly<UseEventsHandler<{ leave: undefined }>>;
  activate(): void;
  deactivate(): void;
  notify(notice: GameNotice): void;
}

/** A game as the arcade lists it, its code loaded the first time it is chosen. */
export interface GameEntry {
  label: string;
  description: string;
  icon: string;
  load: () => Promise<new () => ArcadeGame>;
}
