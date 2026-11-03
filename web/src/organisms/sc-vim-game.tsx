import { createElement, css } from '@jadis/core';
import { JlKbd, JlSelect } from '@syware/jlib';

import { ScElement } from '../base/sc-element';
import { watchTyping } from '../lib/typed-keys';
import { LESSON_ORDER, LESSONS, toolsOf } from '../lib/vim-lessons';
import {
  clearPending,
  efficiency,
  hasPending,
  newRound,
  pauseRound,
  pressKey,
  startRound,
  TARGETS_PER_ROUND,
} from '../lib/vim-round';
import { noticeScreen, ScGameScreen } from '../molecules/sc-game-screen';
import { bestOf, keepBest, keepLesson, lastLesson } from '../services/vim-progress';
import { BASE_STYLES } from '../styles/shared';
import styles from './sc-vim-game.css?inline';

import type { SelectOption } from '@syware/jlib';
import type { GameNotice, GamePhase, GameScreen } from '../types/game.types';
import type { LessonId, VimRound } from '../types/vim.types';

const ESCAPE = 'Escape';
const ENTER = 'Enter';
const MS_PER_SECOND = 1000;

const LESSON_OPTIONS: SelectOption<LessonId>[] = LESSON_ORDER.map((id) => ({
  label: LESSONS[id].name,
  value: id,
}));

const isLesson = (value: string): value is LessonId => LESSON_ORDER.some((id) => id === value);

const SCREENS: Readonly<Record<GamePhase, (round: VimRound, seconds: number) => GameScreen | undefined>> = {
  over: (round, seconds) => ({
    action: 'Again',
    heading: 'Lesson done',
    leave: false,
    text: `${TARGETS_PER_ROUND} targets in ${seconds} s, ${round.totalKeys} keys for a par of ${round.totalPar}: ${efficiency(round)}% · best ${bestOf(round.lesson)}%.`,
  }),
  paused: () => ({ action: 'Resume', heading: 'Paused', leave: false, text: 'Enter goes on too.' }),
  playing: () => undefined,
  ready: (round) => ({
    action: 'Start',
    heading: LESSONS[round.lesson].name,
    leave: false,
    text: `${LESSONS[round.lesson].text} Reach ${TARGETS_PER_ROUND} targets in as few keys as you can.`,
  }),
};

/** A keycap for each character of some keys, as typed: `G` stays a capital. */
const keycaps = (keys: string): Node[] =>
  [...keys].map((char) => {
    const cap = createElement(JlKbd, {});
    cap.textContent = char;
    return cap;
  });

/** The line's number in the gutter: from the top, or how far from the cursor's line, which shows its own. */
const lineNumber = (round: VimRound, row: number): string => {
  const distance = Math.abs(row - round.cursor.row);
  return LESSONS[round.lesson].numbers === 'absolute' || distance === 0 ? String(row + 1) : String(distance);
};

/** A line of the text, the cursor and the target marked on it. */
const lineNode = (round: VimRound, row: number): Node => {
  const line = round.lines[row] ?? '';
  const target = round.target.place.row === row ? round.target.place.col : undefined;
  const cursor = round.cursor.row === row ? round.cursor.col : undefined;
  const marksOf = (col: number): string =>
    [col === target ? 'target' : '', col === cursor ? 'cursor' : ''].join(' ').trim();
  const columns = [...new Set([target, cursor])].filter((col) => col !== undefined).sort((a, b) => a - b);
  const pieces = columns.flatMap((col, index) => [
    line.slice((columns[index - 1] ?? -1) + 1, col),
    <span class={marksOf(col)}>{line[col] ?? ' '}</span>,
  ]);
  return (
    <div class={row === round.cursor.row ? 'line current' : 'line'}>
      <span class="number">{lineNumber(round, row)}</span>
      <span class="text">
        {pieces}
        {line.slice((columns.at(-1) ?? -1) + 1)}
      </span>
    </div>
  );
};

/**
 * Vim's motions as a game: move the cursor to the target in as few keys as the best way there (its par), with the
 * lesson's motions, and see that way once there. The lessons bring a few motions each.
 */
export class ScVimGame extends ScElement {
  static readonly selector = 'sc-vim-game';

  readonly events = this.useEvents<{ leave: undefined }>();

  private _round: VimRound = newRound(lastLesson(), Math.random);
  private _notice: GameNotice | undefined;
  // The time the round ran: what it ran before its last pause, and since when it runs again.
  private _clock: { spent: number; since: number | undefined } = { since: undefined, spent: 0 };

  private readonly _refs = this.useRefs((ref) => ({
    input: ref<HTMLInputElement>('.keys'),
    last: ref('.last'),
    lesson: ref<JlSelect<LessonId>>('.lesson'),
    lessonKeys: ref('.lesson-keys'),
    pending: ref('.pending'),
    progress: ref('.progress'),
    screen: ref(ScGameScreen),
    spent: ref('.spent'),
    stage: ref('.stage'),
    text: ref('.buffer'),
  }));

  templateHtml(): Node {
    return (
      <div class="game">
        <div class="bar">
          <JlSelect<LessonId> class="lesson" size="sm" label="Lesson" options={LESSON_OPTIONS} />
          <span class="progress" />
        </div>
        <div class="stage">
          <div class="buffer" aria-hidden="true" />
          <input class="keys" aria-label="Vim keys" autocomplete="off" autocapitalize="off" spellcheck={false} />
          <ScGameScreen />
        </div>
        <div class="status">
          <span class="pending" role="status" />
          <span class="spent" />
        </div>
        <p class="last" />
        <div class="lesson-keys" />
      </div>
    );
  }

  templateCss(): string {
    return css`${BASE_STYLES}${styles}`;
  }

  onConnect(): void {
    const { input, lesson, screen, stage } = this._refs;
    screen.events.register('action', () => this.go());
    screen.events.register('leave', () => this.events.emit('leave'));
    lesson.events.register('change', (value) => this.changeLesson(value));
    watchTyping(input, this.killSignal, (key) => this.type(key));
    this.on(input, 'keydown', (event) => this.special(event));
    this.on(stage, 'click', () => input.focus());
    this.on(window, 'blur', () => this.pause());
    this.on(document, 'visibilitychange', () => this.pauseWhenHidden());
    this.render();
  }

  onDisconnect(): void {
    this.deactivate();
  }

  activate(): void {
    // Opened again, the person chose to play: what the session wanted then is old news.
    this._notice = undefined;
    this.render();
    this._refs.input.focus();
  }

  deactivate(): void {
    this.pause();
  }

  notify(notice: GameNotice): void {
    this._notice = notice;
    this.pause();
    this.render();
  }

  private type(key: string): void {
    const before = this._round;
    this._round = pressKey(before, key, Math.random);
    this.ended(before);
    this.render();
  }

  /** Escape forgets what was typed toward a motion (else it closes the dialog); Enter starts or resumes. */
  private special(event: KeyboardEvent): void {
    if (event.key === ESCAPE && hasPending(this._round)) {
      event.preventDefault();
      event.stopPropagation();
      this._round = clearPending(this._round);
      this.render();
      return;
    }
    if (event.key !== ENTER || this._round.phase === 'playing') {
      return;
    }
    event.preventDefault();
    this.go();
  }

  /** The round just ended: its time stops and its result is kept. */
  private ended(before: VimRound): void {
    if (before.phase === 'over' || this._round.phase !== 'over') {
      return;
    }
    this.stopClock();
    keepBest(this._round.lesson, efficiency(this._round));
  }

  private go(): void {
    const restart = this._round.phase === 'over';
    this._notice = undefined;
    this._round = startRound(this._round, Math.random);
    this._clock = { since: performance.now(), spent: restart ? 0 : this._clock.spent };
    this.render();
    this._refs.input.focus();
  }

  private pause(): void {
    if (this._round.phase !== 'playing') {
      return;
    }
    this.stopClock();
    this._round = pauseRound(this._round);
    this.render();
  }

  private pauseWhenHidden(): void {
    if (!document.hidden) {
      return;
    }
    this.pause();
  }

  private stopClock(): void {
    const { since, spent } = this._clock;
    this._clock = { since: undefined, spent: since === undefined ? spent : spent + performance.now() - since };
  }

  private changeLesson(value: string): void {
    if (!isLesson(value) || value === this._round.lesson) {
      return;
    }
    keepLesson(value);
    this._round = newRound(value, Math.random);
    this._clock = { since: undefined, spent: 0 };
    this.render();
    this._refs.input.focus();
  }

  private render(): void {
    const round = this._round;
    const { lesson, pending, progress, screen, spent, text } = this._refs;
    lesson.value.set(round.lesson);
    text.replaceChildren(...round.lines.map((_, row) => lineNode(round, row)));
    progress.textContent = `Target ${Math.min(round.reached + 1, TARGETS_PER_ROUND)} of ${TARGETS_PER_ROUND}`;
    pending.textContent = round.refused
      ? `${round.refused} is not a motion of this lesson`
      : `${round.pending.count}${round.pending.awaiting ?? ''}`;
    pending.classList.toggle('refused', round.refused !== undefined);
    spent.textContent = `${round.keys} keys · par ${round.target.cost}`;
    this.renderLast();
    this.renderLessonKeys();
    const seconds = Math.round(this._clock.spent / MS_PER_SECOND);
    screen.show(this._notice ? noticeScreen(this._notice) : SCREENS[round.phase](round, seconds));
  }

  /** How the last target went: its best way, and the keys it took. */
  private renderLast(): void {
    const { last } = this._round;
    if (!last) {
      this._refs.last.replaceChildren('The best way to each target shows here once you reach it.');
      return;
    }
    const verdict = last.keys <= last.par.cost ? 'on par' : `${last.keys} keys`;
    this._refs.last.replaceChildren(
      `Best way: `,
      ...last.par.moves.map((move) => <span class="move">{keycaps(move)}</span>),
      ` par ${last.par.cost}, you: ${verdict}.`
    );
  }

  private renderLessonKeys(): void {
    const { motions, counts } = toolsOf(this._round.lesson);
    const fresh = new Set<string>(LESSONS[this._round.lesson].adds);
    this._refs.lessonKeys.replaceChildren(
      ...[...motions].map((kind) => <span class={fresh.has(kind) ? 'move fresh' : 'move'}>{keycaps(kind)}</span>),
      ...(counts
        ? [<span class={LESSONS[this._round.lesson].counts ? 'move fresh' : 'move'}>{keycaps('1-9')}</span>]
        : [])
    );
  }
}

ScVimGame.register();
