/** Where a game is: its title screen, running, paused, or over with its score. */
export type GamePhase = 'ready' | 'playing' | 'paused' | 'over';

/** What flies at the ship: fast darts, swaying weavers, gunners that aim, rocks that split into pebbles. */
export type EnemyKind = 'dart' | 'weaver' | 'gunner' | 'rock' | 'pebble';

/** What a downed enemy can leave: one more gun, a shield, one more ship. */
export type PickupKind = 'spread' | 'shield' | 'life';

/** How a wave comes in: a line of darts, a file of weavers, gunners, or a rock. */
export type FormationKind = 'darts' | 'weavers' | 'gunners' | 'rock';

/** A random number in [0, 1): `Math.random` in the game, a fixed series in the tests. */
export type Random = () => number;

export interface Point {
  x: number;
  y: number;
}

/** Where the ship heads: the keys' direction (-1 to 1 on each axis), or a point under a finger. */
export type Steering = { kind: 'keys'; x: number; y: number } | { kind: 'point'; x: number; y: number };

/** The colours of the game, read from jlib's tokens: the canvas cannot read custom properties itself. */
export interface GamePalette {
  space: string;
  star: string;
  flash: string;
  ship: string;
  flame: string;
  shot: string;
  enemyShot: string;
  dart: string;
  weaver: string;
  gunner: string;
  rock: string;
  spread: string;
  shield: string;
  life: string;
}

/** A colour of the palette that something is drawn in: everything but the background. */
export type GameTone = Exclude<keyof GamePalette, 'space'>;

export interface Ship extends Point {
  lives: number;
  guns: number;
  /** Seconds before the next shot. */
  reload: number;
  /** Seconds the shield still lasts. */
  shieldFor: number;
  /** Seconds the ship still blinks after a hit, nothing touching it. */
  safeFor: number;
}

export interface Shot extends Point {
  vx: number;
  vy: number;
}

export interface Enemy extends Point {
  id: number;
  kind: EnemyKind;
  hp: number;
  vx: number;
  vy: number;
  /** Seconds since it came in. */
  age: number;
  /** Where a weaver sways around, or the line a gunner holds. */
  anchor: number;
  /** Seconds before a gunner fires again. */
  reload: number;
  /** Seconds it still shows hit. */
  flashFor: number;
}

export interface Pickup extends Point {
  kind: PickupKind;
  age: number;
}

export interface Particle extends Point {
  vx: number;
  vy: number;
  /** Seconds left, out of `span`: it fades as it goes. */
  life: number;
  span: number;
  size: number;
  tone: GameTone;
}

export interface Star extends Point {
  speed: number;
  size: number;
}

export interface GameState {
  phase: GamePhase;
  /** Seconds played: the game gets harder with it. */
  time: number;
  score: number;
  ship: Ship;
  shots: Shot[];
  enemies: Enemy[];
  enemyShots: Shot[];
  pickups: Pickup[];
  particles: Particle[];
  stars: Star[];
  /** Seconds before the next wave. */
  nextWave: number;
  nextId: number;
  /** Seconds the screen still shakes after a hit. */
  shakeFor: number;
}

/** What each kind of enemy is: its size, its strength, its points, its speed and its colour. */
export interface EnemyRule {
  radius: number;
  hp: number;
  score: number;
  speed: number;
  tone: GameTone;
}

/** What the screen over the game says: its heading, a line under it, its button, and a way back to the session. */
export interface GameScreen {
  heading: string;
  text: string;
  action: string;
  leave: boolean;
}

/** Why the game paused on its own: the session's turn ended or waits for the person. */
export interface GameNotice {
  heading: string;
  text: string;
}
