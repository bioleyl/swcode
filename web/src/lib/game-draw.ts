import { ENEMY_RULES, ENEMY_SHOT, FIELD, PICKUP, PICKUP_TONES, SHAKE_TIME } from './game';

import type { Enemy, EnemyKind, GamePalette, GameState, Pickup, PickupKind, Ship } from '../types/game.types';

type Outline = readonly (readonly [number, number])[];

const SHAKE = 4;
// Blinks a second, after a hit and as the shield runs out.
const BLINK_RATE = 14;
const SHIELD = { radius: 19, warning: 2, width: 2 } as const;
const FLAME = { flicker: 3, length: 7, rate: 50 } as const;
const SHOT_SIZE = { height: 12, width: 3 } as const;
const STAR_STREAK = 40;
const PICKUP_PULSE = { amount: 0.12, rate: 6 } as const;
const ROCK = { corners: 9, dent: 0.25 } as const;

const SHIP_OUTLINE: Outline = [
  [0, -14],
  [10, 10],
  [4, 6],
  [-4, 6],
  [-10, 10],
];
const DART_OUTLINE: Outline = [
  [0, 12],
  [-9, -8],
  [0, -3],
  [9, -8],
];
const WEAVER_OUTLINE: Outline = [
  [0, 10],
  [6, 2],
  [13, -6],
  [5, -3],
  [0, -9],
  [-5, -3],
  [-13, -6],
  [-6, 2],
];
const HEXAGON = 6;
const BARREL = { length: 20, width: 5 } as const;

/** The game's colours, from jlib's tokens as `read` gives them: the game follows the theme. */
export const paletteOf = (read: (token: string) => string): GamePalette => ({
  dart: read('--jl-danger'),
  enemyShot: read('--jl-danger'),
  flame: read('--jl-warn'),
  flash: read('--jl-text'),
  gunner: read('--jl-warn'),
  life: read('--jl-accent'),
  rock: read('--jl-muted'),
  shield: read('--jl-code-property'),
  ship: read('--jl-accent'),
  shot: read('--jl-accent-strong'),
  space: read('--jl-bg'),
  spread: read('--jl-ok'),
  star: read('--jl-faint'),
  weaver: read('--jl-code-attribute'),
});

const fillOutline = (context: CanvasRenderingContext2D, outline: Outline, scale = 1): void => {
  context.beginPath();
  for (const [x, y] of outline) {
    context.lineTo(x * scale, y * scale);
  }
  context.closePath();
  context.fill();
};

const fillCircle = (context: CanvasRenderingContext2D, radius: number): void => {
  context.beginPath();
  context.arc(0, 0, radius, 0, Math.PI * 2);
  context.fill();
};

const strokeCircle = (context: CanvasRenderingContext2D, radius: number): void => {
  context.beginPath();
  context.arc(0, 0, radius, 0, Math.PI * 2);
  context.stroke();
};

/** A number in [0, 1) that is always the same for the same rock and corner: its outline keeps its shape. */
const lumpOf = (id: number, corner: number): number => {
  const wave = Math.sin(id * 12.9898 + corner * 78.233) * 43758.5453;
  return wave - Math.floor(wave);
};

const rockOutline = (enemy: Enemy): Outline =>
  Array.from({ length: ROCK.corners }, (_, corner) => {
    const angle = (corner / ROCK.corners) * Math.PI * 2;
    const reach = ENEMY_RULES[enemy.kind].radius * (1 - ROCK.dent + ROCK.dent * lumpOf(enemy.id, corner));
    return [Math.cos(angle) * reach, Math.sin(angle) * reach] as const;
  });

const blinkedOut = (time: number): boolean => Math.floor(time * BLINK_RATE) % 2 === 1;

const drawRock = (context: CanvasRenderingContext2D, enemy: Enemy): void => {
  // Half of them turn one way, half the other.
  context.rotate(enemy.age * (enemy.id % 2 === 0 ? 1 : -1));
  fillOutline(context, rockOutline(enemy));
};

const ENEMY_SHAPES: Readonly<Record<EnemyKind, (context: CanvasRenderingContext2D, enemy: Enemy) => void>> = {
  dart: (context) => fillOutline(context, DART_OUTLINE),
  gunner: (context, enemy) => {
    const radius = ENEMY_RULES[enemy.kind].radius;
    fillOutline(
      context,
      Array.from({ length: HEXAGON }, (_, corner) => {
        const angle = (corner / HEXAGON) * Math.PI * 2;
        return [Math.cos(angle) * radius, Math.sin(angle) * radius] as const;
      })
    );
    context.fillRect(-BARREL.width / 2, 0, BARREL.width, BARREL.length);
  },
  pebble: drawRock,
  rock: drawRock,
  weaver: (context) => fillOutline(context, WEAVER_OUTLINE),
};

const PICKUP_GLYPHS: Readonly<Record<PickupKind, (context: CanvasRenderingContext2D) => void>> = {
  life: (context) => fillOutline(context, SHIP_OUTLINE, 0.4),
  shield: (context) => strokeCircle(context, PICKUP.radius / 2),
  spread: (context) => {
    context.beginPath();
    for (const lean of [-3, 0, 3]) {
      context.moveTo(lean / 2, 3);
      context.lineTo(lean, -4);
    }
    context.stroke();
  },
};

function drawStars(context: CanvasRenderingContext2D, state: GameState, palette: GamePalette): void {
  context.fillStyle = palette.star;
  for (const star of state.stars) {
    context.globalAlpha = star.size / 3;
    context.fillRect(
      star.x,
      star.y,
      star.size * 0.8,
      Math.max(star.size * 0.8, (star.speed / STAR_STREAK) * star.size)
    );
  }
  context.globalAlpha = 1;
}

function drawPickup(context: CanvasRenderingContext2D, pickup: Pickup, palette: GamePalette): void {
  const tone = palette[PICKUP_TONES[pickup.kind]];
  context.save();
  context.translate(pickup.x, pickup.y);
  context.scale(1 + Math.sin(pickup.age * PICKUP_PULSE.rate) * PICKUP_PULSE.amount, 1);
  context.strokeStyle = tone;
  context.fillStyle = tone;
  context.lineWidth = 1.5;
  strokeCircle(context, PICKUP.radius);
  PICKUP_GLYPHS[pickup.kind](context);
  context.restore();
}

function drawEnemy(context: CanvasRenderingContext2D, enemy: Enemy, palette: GamePalette): void {
  context.save();
  context.translate(enemy.x, enemy.y);
  context.fillStyle = enemy.flashFor > 0 ? palette.flash : palette[ENEMY_RULES[enemy.kind].tone];
  ENEMY_SHAPES[enemy.kind](context, enemy);
  context.restore();
}

function drawShots(context: CanvasRenderingContext2D, state: GameState, palette: GamePalette): void {
  context.fillStyle = palette.shot;
  for (const shot of state.shots) {
    context.fillRect(
      shot.x - SHOT_SIZE.width / 2,
      shot.y - SHOT_SIZE.height / 2,
      SHOT_SIZE.width,
      SHOT_SIZE.height
    );
  }
  context.fillStyle = palette.enemyShot;
  for (const shot of state.enemyShots) {
    context.save();
    context.translate(shot.x, shot.y);
    fillCircle(context, ENEMY_SHOT.radius);
    context.restore();
  }
}

function drawShield(context: CanvasRenderingContext2D, ship: Ship, time: number, palette: GamePalette): void {
  if (ship.shieldFor <= 0 || (ship.shieldFor < SHIELD.warning && blinkedOut(time))) {
    return;
  }
  context.strokeStyle = palette.shield;
  context.lineWidth = SHIELD.width;
  strokeCircle(context, SHIELD.radius);
}

function drawShip(context: CanvasRenderingContext2D, state: GameState, palette: GamePalette): void {
  const { ship } = state;
  if (state.phase === 'over' || (ship.safeFor > 0 && blinkedOut(state.time))) {
    return;
  }
  context.save();
  context.translate(ship.x, ship.y);
  context.fillStyle = palette.flame;
  fillOutline(context, [
    [-3.5, 7],
    [3.5, 7],
    [0, 7 + FLAME.length + Math.sin(state.time * FLAME.rate) * FLAME.flicker],
  ]);
  context.fillStyle = palette.ship;
  fillOutline(context, SHIP_OUTLINE);
  drawShield(context, ship, state.time, palette);
  context.restore();
}

function drawParticles(context: CanvasRenderingContext2D, state: GameState, palette: GamePalette): void {
  for (const particle of state.particles) {
    context.globalAlpha = particle.life / particle.span;
    context.fillStyle = palette[particle.tone];
    context.fillRect(particle.x, particle.y, particle.size, particle.size);
  }
  context.globalAlpha = 1;
}

/** Draws the game on a canvas already scaled to the field's units. */
export function drawGame(context: CanvasRenderingContext2D, state: GameState, palette: GamePalette): void {
  context.save();
  context.fillStyle = palette.space;
  context.fillRect(0, 0, FIELD.width, FIELD.height);
  const shake = (state.shakeFor / SHAKE_TIME) * SHAKE;
  context.translate(Math.sin(state.time * 90) * shake, Math.cos(state.time * 70) * shake);
  drawStars(context, state, palette);
  for (const pickup of state.pickups) {
    drawPickup(context, pickup, palette);
  }
  drawShots(context, state, palette);
  for (const enemy of state.enemies) {
    drawEnemy(context, enemy, palette);
  }
  drawShip(context, state, palette);
  drawParticles(context, state, palette);
  context.restore();
}
