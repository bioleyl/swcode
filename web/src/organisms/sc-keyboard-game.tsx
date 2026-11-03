import { css } from '@jadis/core';
import { JlSelect } from '@syware/jlib';

import readme from '../../../README.md?raw';
import { ScElement } from '../base/sc-element';
import {
  focusOf,
  lettersOf,
  letterViews,
  newestOf,
  recordLetter,
  TARGET_WPMS,
  unlockWhenReady,
} from '../lib/keyboard-letters';
import { newRun, nextLine, pauseRun, typeKey } from '../lib/keyboard-run';
import { makeLine, proseOf, wordModelOf } from '../lib/keyboard-words';
import { watchTyping } from '../lib/typed-keys';
import { noticeScreen, ScGameScreen } from '../molecules/sc-game-screen';
import { keepProgress, storedProgress } from '../services/keyboard-progress';
import { BASE_STYLES } from '../styles/shared';
import styles from './sc-keyboard-game.css?inline';

import type { SelectOption } from '@syware/jlib';
import type { GameNotice, GameScreen } from '../types/game.types';
import type {
  KeyboardPhase,
  KeyboardProgress,
  KeyboardRun,
  LetterState,
  LetterView,
  LineResult,
} from '../types/keyboard.types';

// How English words go, learnt from the README: prose that comes with the app, rather than a word list kept for it.
const ENGLISH = wordModelOf(proseOf(readme));
const ENTER = 'Enter';

const TARGET_OPTIONS: SelectOption[] = TARGET_WPMS.map((wpm) => ({ label: `${wpm} wpm`, value: String(wpm) }));

const SCREENS: Readonly<Record<KeyboardPhase, GameScreen | undefined>> = {
  paused: { action: 'Resume', heading: 'Paused', leave: false, text: 'Enter goes on too.' },
  playing: undefined,
  ready: {
    action: 'Start',
    heading: 'Touch typing',
    leave: false,
    text: 'Type the words as they come, eyes off the keys. Six letters to start, and the next once each is typed at the target speed; the slowest comes most often.',
  },
};

const LETTER_HINTS: Readonly<Record<LetterState, (view: LetterView) => string>> = {
  confident: (view) => `${view.letter}: ${view.wpm} wpm, at the target`,
  learning: (view) =>
    view.wpm === undefined ? `${view.letter}: not typed yet` : `${view.letter}: ${view.wpm} wpm`,
  locked: (view) => `${view.letter}: comes later`,
};

const letterNode = (view: LetterView): Node => (
  <span
    class={view.focus ? `letter ${view.state} focus` : `letter ${view.state}`}
    title={LETTER_HINTS[view.state](view)}>
    {view.letter}
  </span>
);

/** The line, what is typed of it dimmed, the character to type marked, and red while a wrong key was typed for it. */
const lineNodes = (run: KeyboardRun): Node[] => [
  <span class="done">{run.line.slice(0, run.at)}</span>,
  <span class={run.missed ? 'at missed' : 'at'}>{run.line[run.at] ?? ''}</span>,
  document.createTextNode(run.line.slice(run.at + 1)),
];

const resultText = (result: LineResult | undefined): string =>
  result ? `Last line: ${result.wpm} wpm · ${result.accuracy}%` : '';

/**
 * Touch typing as a game: lines of words that read like English, made of the letters people have so far. The next
 * letter comes once each of them is typed at the target speed, and the slowest comes most often.
 */
export class ScKeyboardGame extends ScElement {
  static readonly selector = 'sc-keyboard-game';

  readonly events = this.useEvents<{ leave: undefined }>();

  private _phase: KeyboardPhase = 'ready';
  private _progress: KeyboardProgress = storedProgress();
  private _run: KeyboardRun = newRun(this.makeLine(), this.makeLine());
  private _last: LineResult | undefined;
  // The letter just given, said until the next line is typed.
  private _fresh: string | undefined;
  private _notice: GameNotice | undefined;

  private readonly _refs = this.useRefs((ref) => ({
    input: ref<HTMLInputElement>('.keys'),
    letters: ref('.letters'),
    news: ref('.news'),
    next: ref('.next'),
    now: ref('.now'),
    result: ref('.result'),
    screen: ref(ScGameScreen),
    stage: ref('.stage'),
    target: ref<JlSelect>('.target'),
  }));

  templateHtml(): Node {
    return (
      <div class="game">
        <div class="bar">
          <JlSelect class="target" size="sm" label="Target" options={TARGET_OPTIONS} />
          <span class="result" />
        </div>
        <div class="letters" />
        <div class="stage">
          <div class="words" aria-hidden="true">
            <div class="now" />
            <div class="next" />
          </div>
          <input class="keys" aria-label="Typing" autocomplete="off" autocapitalize="off" spellcheck={false} />
          <ScGameScreen />
        </div>
        <p class="news" role="status" />
      </div>
    );
  }

  templateCss(): string {
    return css`${BASE_STYLES}${styles}`;
  }

  onConnect(): void {
    const { input, screen, stage, target } = this._refs;
    screen.events.register('action', () => this.go());
    screen.events.register('leave', () => this.events.emit('leave'));
    target.events.register('change', (value) => this.changeTarget(value));
    watchTyping(input, this.killSignal, (key) => this.type(key));
    this.on(input, 'keydown', (event) => this.enter(event));
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

  private makeLine(): string {
    return makeLine(ENGLISH, new Set(lettersOf(this._progress)), focusOf(this._progress), Math.random);
  }

  private type(key: string): void {
    if (this._phase !== 'playing') {
      return;
    }
    const outcome = typeKey(this._run, key, performance.now());
    this._run = outcome.run;
    this._progress = outcome.typed ? recordLetter(this._progress, outcome.typed) : this._progress;
    this.finishLine(outcome.finished);
    this.renderLine();
  }

  /** A line typed whole: its result shows, the next line comes up, and a letter more once each is fast enough. */
  private finishLine(result: LineResult | undefined): void {
    if (!result) {
      return;
    }
    const unlocked = unlockWhenReady(this._progress);
    this._fresh = unlocked.unlocked > this._progress.unlocked ? newestOf(unlocked) : undefined;
    this._progress = unlocked;
    this._last = result;
    this._run = nextLine(this._run, this.makeLine());
    keepProgress(this._progress);
    this.renderProgress();
  }

  /** Enter starts or resumes. */
  private enter(event: KeyboardEvent): void {
    if (event.key !== ENTER || this._phase === 'playing') {
      return;
    }
    event.preventDefault();
    this.go();
  }

  private go(): void {
    this._notice = undefined;
    this._phase = 'playing';
    this._run = pauseRun(this._run);
    this.render();
    this._refs.input.focus();
  }

  private pause(): void {
    if (this._phase !== 'playing') {
      return;
    }
    this._phase = 'paused';
    this._run = pauseRun(this._run);
    keepProgress(this._progress);
    this.render();
  }

  private pauseWhenHidden(): void {
    if (!document.hidden) {
      return;
    }
    this.pause();
  }

  private changeTarget(value: string): void {
    const wpm = TARGET_WPMS.find((each) => String(each) === value);
    if (wpm === undefined || wpm === this._progress.targetWpm) {
      return;
    }
    this._progress = { ...this._progress, targetWpm: wpm };
    keepProgress(this._progress);
    this.renderProgress();
    this._refs.input.focus();
  }

  private render(): void {
    this.renderLine();
    this.renderProgress();
    this._refs.screen.show(this._notice ? noticeScreen(this._notice) : SCREENS[this._phase]);
  }

  private renderLine(): void {
    this._refs.now.replaceChildren(...lineNodes(this._run));
    this._refs.next.textContent = this._run.next;
  }

  /** The letters, the target, the last line's result, and the letter just given or the one in focus. */
  private renderProgress(): void {
    const { letters, news, result, target } = this._refs;
    target.value.set(String(this._progress.targetWpm));
    letters.replaceChildren(...letterViews(this._progress).map(letterNode));
    result.textContent = resultText(this._last);
    news.textContent = this._fresh
      ? `New letter: ${this._fresh}. It comes often until it is as fast as the others.`
      : `In focus: ${focusOf(this._progress)}, the letter furthest from the target.`;
  }
}

ScKeyboardGame.register();
