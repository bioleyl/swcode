import { describe, expect, it } from 'vitest';

import {
  ENEMY_RULES,
  ENEMY_SHOT,
  FIELD,
  newGame,
  pauseGame,
  SHIP,
  startGame,
  steeringOf,
  stepGame,
} from '../src/lib/game';

import type { Enemy, EnemyKind, GameState, Random, Shot, Steering } from '../src/types/game.types';

// Above every drop chance: nothing drops unless a test says so.
const steady: Random = () => 0.5;
const STILL: Steering = { kind: 'keys', x: 0, y: 0 };
const FRAME = 1 / 60;

const playing = (): GameState => startGame(newGame(steady), steady);

const enemy = (kind: EnemyKind, x: number, y: number): Enemy => ({
  age: 0,
  anchor: x,
  flashFor: 0,
  hp: ENEMY_RULES[kind].hp,
  id: 1,
  kind,
  reload: 99,
  vx: 0,
  vy: 0,
  x,
  y,
});

const shotAt = (x: number, y: number): Shot => ({ vx: 0, vy: 0, x, y });

/** A game with nothing coming for a while: only what the test puts in it. */
const quiet = (state: GameState): GameState => ({ ...state, nextWave: 99 });

const steps = (state: GameState, count: number, steering = STILL): GameState =>
  Array.from({ length: count }).reduce<GameState>((each) => stepGame(each, steering, FRAME, steady), state);

describe('a new game', () => {
  it('waits on its title screen, the ship in the middle with its three lives', () => {
    const game = newGame(steady);
    expect(game.phase).toBe('ready');
    expect(game.ship.lives).toBe(SHIP.lives);
    expect(game.ship.x).toBe(FIELD.width / 2);
  });

  it('sends nothing at the ship and fires nothing before it starts, the stars moving on', () => {
    const game = newGame(steady);
    const later = steps(game, 300);
    expect(later.enemies).toEqual([]);
    expect(later.shots).toEqual([]);
    expect(later.stars).not.toEqual(game.stars);
  });
});

describe('a running game', () => {
  it('fires on its own', () => {
    expect(steps(playing(), 2).shots.length).toBeGreaterThan(0);
  });

  it('stands still while paused', () => {
    const paused = pauseGame(steps(playing(), 10));
    expect(stepGame(paused, { kind: 'keys', x: 1, y: 0 }, FRAME, steady)).toBe(paused);
  });

  it('steers the ship with the keys, inside the field', () => {
    const moved = steps(playing(), 600, { kind: 'keys', x: 1, y: 0 });
    expect(moved.ship.x).toBe(FIELD.width - SHIP.edge);
  });

  it('brings waves in once the first is due', () => {
    expect(steps(playing(), 120).enemies.length).toBeGreaterThan(0);
  });

  it('scores a dart its shot downs', () => {
    const game = { ...quiet(playing()), enemies: [enemy('dart', 100, 100)], shots: [shotAt(100, 100)] };
    const next = stepGame(game, STILL, FRAME, steady);
    expect(next.enemies).toEqual([]);
    expect(next.score).toBe(ENEMY_RULES.dart.score);
  });

  it('breaks a downed rock into pebbles', () => {
    const rock = { ...enemy('rock', 100, 100), hp: 1 };
    const game = { ...quiet(playing()), enemies: [rock], shots: [shotAt(100, 100)] };
    const next = stepGame(game, STILL, FRAME, steady);
    expect(next.enemies.map((each) => each.kind)).toEqual(['pebble', 'pebble', 'pebble']);
  });
});

describe('the ship', () => {
  const underFire = (game: GameState): GameState => ({
    ...quiet(game),
    enemyShots: [{ vx: 0, vy: ENEMY_SHOT.speed, x: game.ship.x, y: game.ship.y }],
  });

  it('loses a ship and a gun when hit, then blinks a while untouched', () => {
    const game = underFire({ ...playing(), ship: { ...playing().ship, guns: 2 } });
    const hit = stepGame(game, STILL, FRAME, steady);
    expect(hit.ship.lives).toBe(SHIP.lives - 1);
    expect(hit.ship.guns).toBe(1);
    expect(stepGame(underFire(hit), STILL, FRAME, steady).ship.lives).toBe(SHIP.lives - 1);
  });

  it('keeps its ship when its shield takes the hit', () => {
    const game = underFire({ ...playing(), ship: { ...playing().ship, shieldFor: 5 } });
    const hit = stepGame(game, STILL, FRAME, steady);
    expect(hit.ship.lives).toBe(SHIP.lives);
    expect(hit.ship.shieldFor).toBe(0);
  });

  it('ends the game with its last ship', () => {
    const game = underFire({ ...playing(), ship: { ...playing().ship, lives: 1 } });
    expect(stepGame(game, STILL, FRAME, steady).phase).toBe('over');
  });

  it('takes one more gun from a pickup, three at most', () => {
    const { ship } = playing();
    const game = {
      ...quiet(playing()),
      pickups: [{ age: 0, kind: 'spread' as const, x: ship.x, y: ship.y }],
      ship: { ...ship, guns: SHIP.maxGuns },
    };
    expect(stepGame(game, STILL, FRAME, steady).ship.guns).toBe(SHIP.maxGuns);
    expect(stepGame({ ...game, ship }, STILL, FRAME, steady).ship.guns).toBe(2);
  });
});

describe('starting again', () => {
  it('starts a lost game anew, under the same stars', () => {
    const lost: GameState = { ...steps(playing(), 30), phase: 'over', score: 1200 };
    const again = startGame(lost, steady);
    expect(again.phase).toBe('playing');
    expect(again.score).toBe(0);
    expect(again.stars).toBe(lost.stars);
  });
});

describe('steeringOf', () => {
  it('goes no faster diagonally than straight', () => {
    const steering = steeringOf(new Set(['ArrowRight', 'KeyW']));
    expect(steering.kind).toBe('keys');
    expect(Math.hypot(steering.x, steering.y)).toBeCloseTo(1);
  });

  it('cancels opposite keys', () => {
    expect(steeringOf(new Set(['ArrowLeft', 'KeyD']))).toEqual({ kind: 'keys', x: 0, y: 0 });
  });

  it('follows a finger over the keys', () => {
    expect(steeringOf(new Set(['ArrowLeft']), { x: 10, y: 20 })).toEqual({ kind: 'point', x: 10, y: 20 });
  });
});
