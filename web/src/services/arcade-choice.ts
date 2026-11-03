import { z } from 'zod';

import { GAME_ORDER } from '../lib/arcade';
import { readStored, writeStored } from './device-store';

import type { GameId } from '../types/arcade.types';

const STORAGE_KEY = 'swcode.arcadeGame';
const FIRST_GAME: GameId = 'drift';

// Without storage, the choice lasts as long as the page.
const memory = { game: readStored<GameId>(STORAGE_KEY, z.enum(GAME_ORDER), FIRST_GAME) };

/** The game people played last on this device, which the arcade opens on. */
export const lastGame = (): GameId => memory.game;

export function keepGame(game: GameId): void {
  memory.game = game;
  writeStored(STORAGE_KEY, game);
}
