import { createElement, css } from '@jadis/core';
import { JlButton, JlIcon } from '@syware/jlib';

import { ScElement } from '../base/sc-element';
import { FIELD, isGameKey, isRunning, newGame, pauseGame, startGame, steeringOf, stepGame } from '../lib/game';
import { drawGame, paletteOf } from '../lib/game-draw';
import { noticeScreen, ScGameScreen } from '../molecules/sc-game-screen';
import { bestScore, keepScore } from '../services/game-score';
import { BASE_STYLES } from '../styles/shared';
import styles from './sc-game.css?inline';

import type { GameNotice, GamePalette, GamePhase, GameScreen, GameState, Point } from '../types/game.types';

// A frame later than this (a tab in the background, a debugger) moves the game this much only.
const MAX_STEP = 1 / 30;
const MS_PER_SECOND = 1000;
// The ship flies this far above the finger that steers it, where the finger does not hide it.
const FINGER_LIFT = 50;
const PAUSE_KEY = 'KeyP';
const START_KEYS: ReadonlySet<string> = new Set(['Space', 'Enter']);
const DARK_QUERY = '(prefers-color-scheme: dark)';

const SCREENS: Readonly<Record<GamePhase, (score: number, best: number) => GameScreen | undefined>> = {
  over: (score, best) => ({
    action: 'Play again',
    heading: 'Game over',
    leave: false,
    text: `Score ${score.toLocaleString('en')} · best ${best.toLocaleString('en')}`,
  }),
  paused: () => ({ action: 'Resume', heading: 'Paused', leave: false, text: 'P goes on too.' }),
  playing: () => undefined,
  ready: () => ({
    action: 'Start',
    heading: 'Clear the sky',
    leave: false,
    text: 'Arrows or WASD steer, or drag on a touch screen. The ship fires on its own; P pauses.',
  }),
};

/**
 * A top-down space shooter to pass the time while the agent works: the ship fires on its own, the waves come
 * faster as it goes. The game runs while `activate`d (its dialog open), and pauses on its own when the tab goes to
 * the background or the session wants the person (`notify`).
 */
export class ScGame extends ScElement {
  static readonly selector = 'sc-game';

  readonly events = this.useEvents<{ leave: undefined }>();

  private _state: GameState = newGame(Math.random);
  private _palette: GamePalette | undefined;
  private _notice: GameNotice | undefined;
  private readonly _keys = new Set<string>();
  private _finger: Point | undefined;
  private _frame = 0;
  private _last = 0;
  private _resize: ResizeObserver | undefined;

  private readonly _refs = this.useRefs((ref) => ({
    canvas: ref('canvas'),
    lives: ref('.lives'),
    pause: ref<JlButton>('.pause'),
    score: ref('.score'),
    screen: ref(ScGameScreen),
    stage: ref('.stage'),
  }));

  templateHtml(): Node {
    return (
      // biome-ignore lint/a11y/noNoninteractiveTabindex: the game takes the keys, so it takes the focus
      <div class="stage" role="application" tabIndex={0} aria-label="Space shooter">
        <canvas />
        <div class="hud">
          <span class="score" />
          <span class="lives" />
          <JlButton class="pause" icon="pause" hint="Pause" variant="ghost" size="sm" hidden />
        </div>
        <ScGameScreen />
      </div>
    );
  }

  templateCss(): string {
    return css`${BASE_STYLES}${styles}`;
  }

  onConnect(): void {
    const { canvas, stage } = this._refs;
    this._refs.screen.events.register('action', () => this.go());
    this._refs.screen.events.register('leave', () => this.events.emit('leave'));
    this._refs.pause.events.register('press', () => this.pause());
    this.on(stage, 'keydown', (event) => this.keyDown(event));
    this.on(stage, 'keyup', (event) => this._keys.delete(event.code));
    this.on(canvas, 'pointerdown', (event) => this.touch(event));
    this.on(canvas, 'pointermove', (event) => this.drag(event));
    this.on(canvas, 'pointerup', () => this.lift());
    this.on(canvas, 'pointercancel', () => this.lift());
    this.on(window, 'blur', () => this.pause());
    this.on(document, 'visibilitychange', () => this.pauseWhenHidden());
    window
      .matchMedia(DARK_QUERY)
      .addEventListener('change', () => this.readPalette(), { signal: this.killSignal });
    this._resize = new ResizeObserver(() => this.fit());
    this._resize.observe(canvas);
    this.renderHud();
    this.renderScreen();
  }

  onDisconnect(): void {
    this._resize?.disconnect();
    this.deactivate();
  }

  /** Its dialog opened: the game draws, and takes the keys. */
  activate(): void {
    // Opened again, the person chose to play: what the session wanted then is old news.
    this._notice = undefined;
    this.readPalette();
    this.renderScreen();
    this._refs.stage.focus();
    cancelAnimationFrame(this._frame);
    this._last = performance.now();
    this._frame = requestAnimationFrame((now) => this.frame(now));
  }

  /** Its dialog closed: the game pauses and stops drawing. */
  deactivate(): void {
    this.pause();
    cancelAnimationFrame(this._frame);
  }

  /** The session wants the person: a running game pauses, and the screen says why. */
  notify(notice: GameNotice): void {
    this._notice = notice;
    this._state = pauseGame(this._state);
    this.renderScreen();
  }

  private frame(now: number): void {
    const dt = Math.min(MAX_STEP, (now - this._last) / MS_PER_SECOND);
    this._last = now;
    this.advance(dt);
    this._frame = requestAnimationFrame((next) => this.frame(next));
  }

  private advance(dt: number): void {
    const previous = this._state;
    this._state = stepGame(previous, steeringOf(this._keys, this._finger), dt, Math.random);
    this.draw();
    this.renderHud(previous);
    if (previous.phase === this._state.phase) {
      return;
    }
    this.keepLostScore();
    this.renderScreen();
  }

  /** A game just lost keeps its score, before its screen shows the best. */
  private keepLostScore(): void {
    if (this._state.phase !== 'over') {
      return;
    }
    keepScore(this._state.score);
  }

  /** The screen's button: start, go on, or play again. */
  private go(): void {
    this._notice = undefined;
    this._state = startGame(this._state, Math.random);
    this.renderHud();
    this.renderScreen();
    // The button hides with its screen: the keys go to the game.
    this._refs.stage.focus();
  }

  private pause(): void {
    this._keys.clear();
    this._finger = undefined;
    if (!isRunning(this._state.phase)) {
      return;
    }
    this._state = pauseGame(this._state);
    this.renderScreen();
  }

  private pauseWhenHidden(): void {
    if (!document.hidden) {
      return;
    }
    this.pause();
  }

  private keyDown(event: KeyboardEvent): void {
    if (isGameKey(event.code)) {
      event.preventDefault();
      this._keys.add(event.code);
      return;
    }
    if (event.code === PAUSE_KEY) {
      this.togglePause();
      return;
    }
    // Space and Enter on the screen's button press it already.
    if (!START_KEYS.has(event.code) || event.target !== this._refs.stage || isRunning(this._state.phase)) {
      return;
    }
    event.preventDefault();
    this.go();
  }

  private togglePause(): void {
    if (isRunning(this._state.phase)) {
      this.pause();
      return;
    }
    if (this._state.phase !== 'paused') {
      return;
    }
    this.go();
  }

  private fieldPoint(event: PointerEvent): Point {
    const box = this._refs.canvas.getBoundingClientRect();
    return {
      x: ((event.clientX - box.left) / box.width) * FIELD.width,
      y: ((event.clientY - box.top) / box.height) * FIELD.height - FINGER_LIFT,
    };
  }

  private touch(event: PointerEvent): void {
    if (!isRunning(this._state.phase)) {
      return;
    }
    this._refs.canvas.setPointerCapture(event.pointerId);
    this._finger = this.fieldPoint(event);
  }

  private drag(event: PointerEvent): void {
    if (!this._finger) {
      return;
    }
    this._finger = this.fieldPoint(event);
  }

  private lift(): void {
    this._finger = undefined;
  }

  private readPalette(): void {
    const style = getComputedStyle(this);
    this._palette = paletteOf((token) => style.getPropertyValue(token).trim());
    this.draw();
  }

  /** The canvas as many pixels as it shows, each field unit as many of them as fit. */
  private fit(): void {
    const { canvas } = this._refs;
    const box = canvas.getBoundingClientRect();
    canvas.width = Math.round(box.width * devicePixelRatio);
    canvas.height = Math.round(box.height * devicePixelRatio);
    this.draw();
  }

  private draw(): void {
    const { canvas } = this._refs;
    const context = canvas.getContext('2d');
    if (!context || !this._palette || canvas.width === 0) {
      return;
    }
    // The pixels the field does not reach, a rounding at its foot, would keep every frame drawn there.
    context.setTransform(1, 0, 0, 1, 0, 0);
    context.clearRect(0, 0, canvas.width, canvas.height);
    const scale = canvas.width / FIELD.width;
    context.setTransform(scale, 0, 0, scale, 0, 0);
    drawGame(context, this._state, this._palette);
  }

  /** The score and the ships left, when they changed since the `previous` frame. */
  private renderHud(previous?: GameState): void {
    const { lives, score } = this._refs;
    if (previous && previous.score === this._state.score && previous.ship.lives === this._state.ship.lives) {
      return;
    }
    score.textContent = this._state.score.toLocaleString('en');
    lives.replaceChildren(
      ...Array.from({ length: this._state.ship.lives }, () =>
        createElement(JlIcon, { props: { name: 'rocket', size: 14 } })
      )
    );
    lives.title = `${this._state.ship.lives} ships left`;
  }

  private renderScreen(): void {
    const screen = this._notice
      ? noticeScreen(this._notice)
      : SCREENS[this._state.phase](this._state.score, bestScore());
    this._refs.pause.hidden = !isRunning(this._state.phase);
    this._refs.screen.show(screen);
  }
}

ScGame.register();
