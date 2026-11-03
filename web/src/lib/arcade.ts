import type { GameId } from '../types/arcade.types';

/** The games in the order their tabs come. */
export const GAME_ORDER = ['drift', 'motions', 'keyboard'] as const satisfies readonly GameId[];

export const isGameId = (value: string): value is GameId => GAME_ORDER.some((id) => id === value);
