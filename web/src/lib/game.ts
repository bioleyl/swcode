import { between, pickWeighted } from './random';

import type {
  Enemy,
  EnemyKind,
  EnemyRule,
  FormationKind,
  GamePhase,
  GameState,
  GameTone,
  Particle,
  Pickup,
  PickupKind,
  Point,
  Random,
  Ship,
  Shot,
  Star,
  Steering,
} from '../types/game.types';

/** The playing field, in its own units: the canvas scales it to its size. */
export const FIELD = { height: 540, width: 360 } as const;

export const SHIP = {
  // How far from the field's sides and bottom the ship stays.
  edge: 14,
  // How fast the ship closes on a finger: the part of the gap it covers in a second, more or less.
  follow: 12,
  hitGrace: 2,
  lives: 3,
  maxGuns: 3,
  maxLives: 5,
  radius: 11,
  reload: 0.16,
  shieldGrace: 1,
  shieldTime: 10,
  speed: 250,
  startY: 470,
} as const;

export const SHOT = { radius: 3, speed: 560 } as const;
export const ENEMY_SHOT = { radius: 4, speed: 190 } as const;
export const PICKUP = { fall: 70, radius: 9, score: 50 } as const;

export const ENEMY_RULES: Readonly<Record<EnemyKind, EnemyRule>> = {
  dart: { hp: 1, radius: 10, score: 100, speed: 170, tone: 'dart' },
  gunner: { hp: 5, radius: 16, score: 300, speed: 80, tone: 'gunner' },
  pebble: { hp: 1, radius: 9, score: 50, speed: 100, tone: 'rock' },
  rock: { hp: 7, radius: 21, score: 200, speed: 45, tone: 'rock' },
  weaver: { hp: 2, radius: 12, score: 150, speed: 85, tone: 'weaver' },
};

export const PICKUP_TONES: Readonly<Record<PickupKind, GameTone>> = {
  life: 'life',
  shield: 'shield',
  spread: 'spread',
};

// Each gun of the ship: how far beside its nose it fires, and how much sideways.
const GUN_PATTERNS: readonly (readonly { dx: number; vx: number }[])[] = [
  [{ dx: 0, vx: 0 }],
  [
    { dx: -5, vx: 0 },
    { dx: 5, vx: 0 },
  ],
  [
    { dx: 0, vx: 0 },
    { dx: -6, vx: -110 },
    { dx: 6, vx: 110 },
  ],
];

// Every this many seconds, the game is one level harder: more waves, faster enemies.
const LEVEL_SECONDS = 45;
const PACE_PER_LEVEL = 0.12;
const WAVE = { first: 1.2, gap: 2.6, gapPerLevel: 0.25, minGap: 0.9 } as const;
// How far out of the field something may go before it is dropped.
const MARGIN = 40;
const WEAVE = { amplitude: 55, rate: 2.4 } as const;
const GUNNER = { firstReload: 1.2, holdFrom: 70, holdSpread: 60, reload: 1.7, stay: 9, strafe: 60 } as const;
const FLASH_TIME = 0.08;
export const SHAKE_TIME = 0.3;
const STARS = { count: 60, maxSpeed: 90, minSpeed: 15, sizes: 3 } as const;
const BURST = { enemy: 14, ship: 40, spark: 3, speed: 160 } as const;
const PARTICLE_LIFE = { max: 0.9, min: 0.3 } as const;
const MAX_PARTICLES = 400;

// The pieces a downed enemy breaks into.
const SPLITS: Readonly<Record<EnemyKind, number>> = { dart: 0, gunner: 0, pebble: 0, rock: 3, weaver: 0 };
const PEBBLE_SPREAD = 50;

const DROP_CHANCES: Readonly<Record<EnemyKind, number>> = {
  dart: 0.05,
  gunner: 0.35,
  pebble: 0.03,
  rock: 0.25,
  weaver: 0.08,
};
const PICKUP_WEIGHTS: Readonly<Record<PickupKind, number>> = { life: 1, shield: 3, spread: 5 };

const PICKUP_EFFECTS: Readonly<Record<PickupKind, (ship: Ship) => Ship>> = {
  life: (ship) => ({ ...ship, lives: Math.min(SHIP.maxLives, ship.lives + 1) }),
  shield: (ship) => ({ ...ship, shieldFor: SHIP.shieldTime }),
  spread: (ship) => ({ ...ship, guns: Math.min(SHIP.maxGuns, ship.guns + 1) }),
};

// Where a keyboard key steers, by its place on the keyboard (`code`): WASD stays put on any layout.
const KEY_DIRECTIONS: ReadonlyMap<string, Point> = new Map([
  ['ArrowLeft', { x: -1, y: 0 }],
  ['ArrowRight', { x: 1, y: 0 }],
  ['ArrowUp', { x: 0, y: -1 }],
  ['ArrowDown', { x: 0, y: 1 }],
  ['KeyA', { x: -1, y: 0 }],
  ['KeyD', { x: 1, y: 0 }],
  ['KeyW', { x: 0, y: -1 }],
  ['KeyS', { x: 0, y: 1 }],
]);

/** What a formation brings in, before it has its ids and its pace. */
interface EnemyPlan extends Point {
  kind: EnemyKind;
  vx?: number;
  anchor?: number;
}

const clamp = (value: number, low: number, high: number): number => Math.min(high, Math.max(low, value));
const levelAt = (time: number): number => 1 + time / LEVEL_SECONDS;
const paceAt = (level: number): number => 1 + (level - 1) * PACE_PER_LEVEL;
const waveGap = (level: number): number => Math.max(WAVE.minGap, WAVE.gap - level * WAVE.gapPerLevel);

const touches = (a: Point, aRadius: number, b: Point, bRadius: number): boolean =>
  Math.hypot(a.x - b.x, a.y - b.y) < aRadius + bRadius;

const FORMATION_WEIGHTS = (level: number): Readonly<Record<FormationKind, number>> => ({
  darts: 3,
  gunners: 0.6 + level * 0.4,
  rock: 1.2,
  weavers: 2.5,
});

const FORMATIONS: Readonly<Record<FormationKind, (random: Random, level: number) => EnemyPlan[]>> = {
  // A V of darts, wider as the game goes.
  darts: (random, level) => {
    const count = Math.min(7, 3 + Math.floor(level));
    const middle = between(random, 80, FIELD.width - 80);
    return Array.from({ length: count }, (_, index) => {
      const offset = index - (count - 1) / 2;
      return { kind: 'dart', vx: 0, x: middle + offset * 28, y: -20 - Math.abs(offset) * 24 };
    });
  },
  gunners: (random, level) =>
    Array.from({ length: level > 2.5 ? 2 : 1 }, (_, index) => ({
      anchor: GUNNER.holdFrom + random() * GUNNER.holdSpread,
      kind: 'gunner',
      vx: (index % 2 === 0 ? 1 : -1) * GUNNER.strafe,
      x: between(random, 50, FIELD.width - 50),
      y: -30,
    })),
  rock: (random) => [
    { kind: 'rock', vx: between(random, -30, 30), x: between(random, 60, FIELD.width - 60), y: -30 },
  ],
  // A file of weavers swaying around the same line.
  weavers: (random) => {
    const anchor = between(random, 80, FIELD.width - 80);
    return Array.from({ length: 5 }, (_, index) => ({ anchor, kind: 'weaver', x: anchor, y: -20 - index * 40 }));
  },
};

const makeEnemy = (plan: EnemyPlan, id: number, pace: number): Enemy => ({
  age: 0,
  anchor: plan.anchor ?? plan.x,
  flashFor: 0,
  hp: ENEMY_RULES[plan.kind].hp,
  id,
  kind: plan.kind,
  reload: GUNNER.firstReload,
  vx: plan.vx ?? 0,
  vy: ENEMY_RULES[plan.kind].speed * pace,
  x: plan.x,
  y: plan.y,
});

const starAt = (y: number, random: Random): Star => ({
  size: 1 + Math.floor(random() * STARS.sizes),
  speed: between(random, STARS.minSpeed, STARS.maxSpeed),
  x: random() * FIELD.width,
  y,
});

const newShip = (): Ship => ({
  guns: 1,
  lives: SHIP.lives,
  reload: 0,
  safeFor: 0,
  shieldFor: 0,
  x: FIELD.width / 2,
  y: SHIP.startY,
});

/** A game on its title screen, the stars already out. */
export const newGame = (random: Random): GameState => ({
  enemies: [],
  enemyShots: [],
  nextId: 1,
  nextWave: WAVE.first,
  particles: [],
  phase: 'ready',
  pickups: [],
  score: 0,
  shakeFor: 0,
  ship: newShip(),
  shots: [],
  stars: Array.from({ length: STARS.count }, () => starAt(random() * FIELD.height, random)),
  time: 0,
});

/** The start button: the title screen's or a paused game goes on, a lost one starts again under the same stars. */
export const startGame = (state: GameState, random: Random): GameState =>
  state.phase === 'over'
    ? { ...newGame(random), phase: 'playing', stars: state.stars }
    : { ...state, phase: 'playing' };

/** Pauses a running game; any other stays as it is. */
export const pauseGame = (state: GameState): GameState =>
  state.phase === 'playing' ? { ...state, phase: 'paused' } : state;

/** Whether the game runs: the ship flies and the waves come. */
export const isRunning = (phase: GamePhase): boolean => phase === 'playing';

/** Where the keys held down steer the ship, or the finger on the field when there is one. */
export const steeringOf = (keys: ReadonlySet<string>, finger?: Point): Steering => {
  if (finger) {
    return { kind: 'point', x: finger.x, y: finger.y };
  }
  const directions = [...keys].flatMap((key) => KEY_DIRECTIONS.get(key) ?? []);
  const x = clamp(
    directions.reduce((sum, direction) => sum + direction.x, 0),
    -1,
    1
  );
  const y = clamp(
    directions.reduce((sum, direction) => sum + direction.y, 0),
    -1,
    1
  );
  // Diagonally, no faster than straight.
  const length = Math.max(1, Math.hypot(x, y));
  return { kind: 'keys', x: x / length, y: y / length };
};

/** Whether a key is one the game uses: the page does not scroll or move its focus on those. */
export const isGameKey = (code: string): boolean => KEY_DIRECTIONS.has(code);

const burst = (at: Point, tone: GameTone, count: number, random: Random): Particle[] =>
  Array.from({ length: count }, () => {
    const angle = random() * Math.PI * 2;
    const speed = random() * BURST.speed;
    const life = between(random, PARTICLE_LIFE.min, PARTICLE_LIFE.max);
    return {
      life,
      size: 1 + random() * 2,
      span: life,
      tone,
      vx: Math.cos(angle) * speed,
      vy: Math.sin(angle) * speed,
      x: at.x,
      y: at.y,
    };
  });

const moveShot =
  (dt: number) =>
  <Moving extends Shot>(shot: Moving): Moving => ({ ...shot, x: shot.x + shot.vx * dt, y: shot.y + shot.vy * dt });

const onField = (point: Point): boolean =>
  point.x > -MARGIN && point.x < FIELD.width + MARGIN && point.y > -MARGIN && point.y < FIELD.height + MARGIN;

// Enemies come in from above the field: only those gone past its bottom or its sides are dropped.
const inPlay = (enemy: Enemy): boolean =>
  enemy.y < FIELD.height + MARGIN && enemy.x > -MARGIN && enemy.x < FIELD.width + MARGIN;

const moveParticles = (particles: readonly Particle[], dt: number): Particle[] =>
  particles
    .map((particle) => ({
      ...particle,
      life: particle.life - dt,
      x: particle.x + particle.vx * dt,
      y: particle.y + particle.vy * dt,
    }))
    .filter((particle) => particle.life > 0);

const moveStars = (stars: readonly Star[], dt: number, random: Random): Star[] =>
  stars.map((star) => {
    const y = star.y + star.speed * dt;
    return y > FIELD.height ? starAt(y - FIELD.height, random) : { ...star, y };
  });

/** Bounces off the field's sides: rocks and gunners stay in it. */
const bounce = (enemy: Enemy): Enemy => {
  const radius = ENEMY_RULES[enemy.kind].radius;
  const out = enemy.x < radius || enemy.x > FIELD.width - radius;
  return out ? { ...enemy, vx: -enemy.vx, x: clamp(enemy.x, radius, FIELD.width - radius) } : enemy;
};

const drift = (enemy: Enemy, dt: number): Enemy =>
  bounce({ ...enemy, x: enemy.x + enemy.vx * dt, y: enemy.y + enemy.vy * dt });

const ENEMY_MOVES: Readonly<Record<EnemyKind, (enemy: Enemy, dt: number) => Enemy>> = {
  dart: drift,
  // Comes down to its line, strafes along it, and goes on down once it has stayed long enough.
  gunner: (enemy, dt) => {
    const leaving = enemy.age > GUNNER.stay;
    const y = enemy.y + enemy.vy * dt;
    return bounce({ ...enemy, x: enemy.x + enemy.vx * dt, y: leaving ? y : Math.min(enemy.anchor, y) });
  },
  pebble: drift,
  rock: drift,
  weaver: (enemy, dt) => ({
    ...enemy,
    x: enemy.anchor + Math.sin(enemy.age * WEAVE.rate + enemy.id) * WEAVE.amplitude,
    y: enemy.y + enemy.vy * dt,
  }),
};

const moveEnemy = (enemy: Enemy, dt: number): Enemy =>
  ENEMY_MOVES[enemy.kind](
    { ...enemy, age: enemy.age + dt, flashFor: Math.max(0, enemy.flashFor - dt), reload: enemy.reload - dt },
    dt
  );

const STEERS: Readonly<Record<Steering['kind'], (ship: Ship, target: Point, dt: number) => Point>> = {
  keys: (ship, direction, dt) => ({
    x: ship.x + direction.x * SHIP.speed * dt,
    y: ship.y + direction.y * SHIP.speed * dt,
  }),
  point: (ship, finger, dt) => {
    const share = Math.min(1, SHIP.follow * dt);
    return { x: ship.x + (finger.x - ship.x) * share, y: ship.y + (finger.y - ship.y) * share };
  },
};

const moveShip = (ship: Ship, steering: Steering, dt: number): Ship => {
  const place = STEERS[steering.kind](ship, steering, dt);
  return {
    ...ship,
    safeFor: Math.max(0, ship.safeFor - dt),
    shieldFor: Math.max(0, ship.shieldFor - dt),
    x: clamp(place.x, SHIP.edge, FIELD.width - SHIP.edge),
    y: clamp(place.y, SHIP.edge, FIELD.height - SHIP.edge),
  };
};

/** The ship fires on its own, from each of its guns, as soon as it has reloaded. */
const fire = (ship: Ship, dt: number): { ship: Ship; shots: Shot[] } => {
  const reload = ship.reload - dt;
  if (reload > 0) {
    return { ship: { ...ship, reload }, shots: [] };
  }
  const guns = GUN_PATTERNS[ship.guns - 1] ?? GUN_PATTERNS[0] ?? [];
  return {
    ship: { ...ship, reload: reload + SHIP.reload },
    shots: guns.map((gun) => ({ vx: gun.vx, vy: -SHOT.speed, x: ship.x + gun.dx, y: ship.y - SHIP.radius })),
  };
};

const aimAt = (from: Point, to: Point): Shot => {
  const length = Math.hypot(to.x - from.x, to.y - from.y) || 1;
  return {
    vx: ((to.x - from.x) / length) * ENEMY_SHOT.speed,
    vy: ((to.y - from.y) / length) * ENEMY_SHOT.speed,
    x: from.x,
    y: from.y,
  };
};

const firesNow = (enemy: Enemy): boolean =>
  enemy.kind === 'gunner' && enemy.reload <= 0 && enemy.y > 0 && enemy.age < GUNNER.stay;

/** Gunners on the field aim at the ship, more often as the game goes. */
const gunfire = (state: GameState, level: number): GameState => {
  const shooters = state.enemies.filter(firesNow);
  if (shooters.length === 0) {
    return state;
  }
  return {
    ...state,
    enemies: state.enemies.map((enemy) =>
      firesNow(enemy) ? { ...enemy, reload: GUNNER.reload / Math.sqrt(level) } : enemy
    ),
    enemyShots: [...state.enemyShots, ...shooters.map((enemy) => aimAt(enemy, state.ship))],
  };
};

/** What a downed rock breaks into: pebbles spreading as they fall. */
const piecesOf = (enemy: Enemy): EnemyPlan[] =>
  Array.from({ length: SPLITS[enemy.kind] }, (_, index) => ({
    kind: 'pebble',
    vx: (index - (SPLITS[enemy.kind] - 1) / 2) * PEBBLE_SPREAD,
    x: enemy.x,
    y: enemy.y,
  }));

const dropsOf = (enemy: Enemy, random: Random): Pickup[] =>
  random() < DROP_CHANCES[enemy.kind]
    ? [{ age: 0, kind: pickWeighted(PICKUP_WEIGHTS, random), x: enemy.x, y: enemy.y }]
    : [];

/** The ship's shots against the enemies: each shot hits one, the downed ones burst, score, break up and drop. */
const strikeEnemies = (state: GameState, random: Random): GameState => {
  const spent = new Set<Shot>();
  const struck = state.enemies.map((enemy) => {
    const hits = state.shots.filter(
      (shot) => !spent.has(shot) && touches(shot, SHOT.radius, enemy, ENEMY_RULES[enemy.kind].radius)
    );
    for (const shot of hits) {
      spent.add(shot);
    }
    return hits.length === 0 ? enemy : { ...enemy, flashFor: FLASH_TIME, hp: enemy.hp - hits.length };
  });
  const downed = struck.filter((enemy) => enemy.hp <= 0);
  const pieces = downed.flatMap(piecesOf);
  return {
    ...state,
    enemies: [
      ...struck.filter((enemy) => enemy.hp > 0),
      ...pieces.map((piece, index) => makeEnemy(piece, state.nextId + index, 1)),
    ],
    nextId: state.nextId + pieces.length,
    particles: [
      ...state.particles,
      ...[...spent].flatMap((shot) => burst(shot, 'shot', BURST.spark, random)),
      ...downed.flatMap((enemy) => burst(enemy, ENEMY_RULES[enemy.kind].tone, BURST.enemy, random)),
    ],
    pickups: [...state.pickups, ...downed.flatMap((enemy) => dropsOf(enemy, random))],
    score: state.score + downed.reduce((sum, enemy) => sum + ENEMY_RULES[enemy.kind].score, 0),
    shots: state.shots.filter((shot) => !spent.has(shot)),
  };
};

/** A hit: the shield takes it if there is one, else a ship and a gun are lost. Either way the ship blinks a while. */
const hurt = (ship: Ship): Ship =>
  ship.shieldFor > 0
    ? { ...ship, safeFor: SHIP.shieldGrace, shieldFor: 0 }
    : { ...ship, guns: Math.max(1, ship.guns - 1), lives: ship.lives - 1, safeFor: SHIP.hitGrace };

/** Enemies and their shots against the ship: what touches it is gone, and the ship is hurt. */
const strikeShip = (state: GameState, random: Random): GameState => {
  const { ship } = state;
  if (ship.safeFor > 0) {
    return state;
  }
  const rammed = state.enemies.filter((enemy) =>
    touches(enemy, ENEMY_RULES[enemy.kind].radius, ship, SHIP.radius)
  );
  const shot = state.enemyShots.filter((each) => touches(each, ENEMY_SHOT.radius, ship, SHIP.radius));
  if (rammed.length === 0 && shot.length === 0) {
    return state;
  }
  return {
    ...state,
    enemies: state.enemies.filter((enemy) => !rammed.includes(enemy)),
    enemyShots: state.enemyShots.filter((each) => !shot.includes(each)),
    particles: [
      ...state.particles,
      ...rammed.flatMap((enemy) => burst(enemy, ENEMY_RULES[enemy.kind].tone, BURST.enemy, random)),
      ...burst(ship, ship.shieldFor > 0 ? 'shield' : 'ship', BURST.enemy, random),
    ],
    shakeFor: SHAKE_TIME,
    ship: hurt(ship),
  };
};

const collect = (state: GameState, random: Random): GameState => {
  const taken = state.pickups.filter((pickup) => touches(pickup, PICKUP.radius, state.ship, SHIP.radius));
  if (taken.length === 0) {
    return state;
  }
  return {
    ...state,
    particles: [
      ...state.particles,
      ...taken.flatMap((pickup) => burst(pickup, PICKUP_TONES[pickup.kind], BURST.enemy, random)),
    ],
    pickups: state.pickups.filter((pickup) => !taken.includes(pickup)),
    score: state.score + taken.length * PICKUP.score,
    ship: taken.reduce((ship, pickup) => PICKUP_EFFECTS[pickup.kind](ship), state.ship),
  };
};

const spawnWaves = (state: GameState, dt: number, random: Random, level: number): GameState => {
  const nextWave = state.nextWave - dt;
  if (nextWave > 0) {
    return { ...state, nextWave };
  }
  const plans = FORMATIONS[pickWeighted(FORMATION_WEIGHTS(level), random)](random, level);
  return {
    ...state,
    enemies: [
      ...state.enemies,
      ...plans.map((plan, index) => makeEnemy(plan, state.nextId + index, paceAt(level))),
    ],
    nextId: state.nextId + plans.length,
    nextWave: waveGap(level),
  };
};

const endIfLost = (state: GameState, random: Random): GameState =>
  state.ship.lives > 0
    ? state
    : {
        ...state,
        particles: [
          ...state.particles,
          ...burst(state.ship, 'ship', BURST.ship, random),
          ...burst(state.ship, 'flame', BURST.ship, random),
        ],
        phase: 'over',
      };

const play = (state: GameState, steering: Steering, dt: number, random: Random): GameState => {
  const level = levelAt(state.time);
  const fired = fire(moveShip(state.ship, steering, dt), dt);
  const moved: GameState = {
    ...state,
    enemies: state.enemies.map((enemy) => moveEnemy(enemy, dt)).filter(inPlay),
    enemyShots: state.enemyShots.map(moveShot(dt)).filter(onField),
    pickups: state.pickups
      .map((pickup) => ({ ...pickup, age: pickup.age + dt, y: pickup.y + PICKUP.fall * dt }))
      .filter(onField),
    ship: fired.ship,
    shots: [...state.shots, ...fired.shots].map(moveShot(dt)).filter(onField),
    time: state.time + dt,
  };
  const played = [
    (each: GameState) => gunfire(each, level),
    (each: GameState) => strikeEnemies(each, random),
    (each: GameState) => strikeShip(each, random),
    (each: GameState) => collect(each, random),
    (each: GameState) => spawnWaves(each, dt, random, level),
    (each: GameState) => endIfLost(each, random),
  ].reduce((each, phase) => phase(each), moved);
  return { ...played, particles: played.particles.slice(-MAX_PARTICLES) };
};

/**
 * The game `dt` seconds later. A paused game stands still; on the title screen and once over, only the stars and
 * the last bursts move.
 */
export const stepGame = (state: GameState, steering: Steering, dt: number, random: Random): GameState => {
  if (state.phase === 'paused') {
    return state;
  }
  const scenery = {
    ...state,
    particles: moveParticles(state.particles, dt),
    shakeFor: Math.max(0, state.shakeFor - dt),
    stars: moveStars(state.stars, dt, random),
  };
  return isRunning(state.phase) ? play(scenery, steering, dt, random) : scenery;
};
