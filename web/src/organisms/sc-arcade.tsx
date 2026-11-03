import { css } from '@jadis/core';
import { JlDialog, JlLoadState, JlTabPanel, JlTabs } from '@syware/jlib';

import { ScElement } from '../base/sc-element';
import { GAME_ORDER, isGameId } from '../lib/arcade';
import { keepGame, lastGame } from '../services/arcade-choice';
import { BASE_STYLES } from '../styles/shared';
import styles from './sc-arcade.css?inline';

import type { SessionStatus } from '../../../shared/protocol/session.schema';
import type { ArcadeGame, GameEntry, GameId } from '../types/arcade.types';
import type { GameNotice } from '../types/game.types';

// Each game in a chunk of its own, loaded the first time its tab opens.
const GAMES: Readonly<Record<GameId, GameEntry>> = {
  drift: {
    description: 'A space shooter: steer, and the ship fires on its own.',
    icon: 'rocket',
    label: 'Drift',
    load: () => import('./sc-game').then((module) => module.ScGame),
  },
  keyboard: {
    description: 'Touch typing, a letter at a time: the next comes once each is fast enough.',
    icon: 'keyboard',
    label: 'Keyboard',
    load: () => import('./sc-keyboard-game').then((module) => module.ScKeyboardGame),
  },
  motions: {
    description: "Vim's motions: reach each target in as few keys as the best way there.",
    icon: 'terminal',
    label: 'Vim',
    load: () => import('./sc-vim-game').then((module) => module.ScVimGame),
  },
};

// What pauses the game while a turn runs: the turn ended, or it waits for the person.
const NOTICES: Readonly<Record<SessionStatus, GameNotice | undefined>> = {
  idle: { heading: 'swcode is done', text: 'Its turn is over: it waits for your next message.' },
  waiting: { heading: 'swcode needs you', text: 'It asks you first: an approval, or a question.' },
  working: undefined,
};

/**
 * The games, in a dialog over the session, a tab each: the one shown runs while the dialog is open, and pauses
 * when the session's turn ends or waits for the person.
 */
export class ScArcade extends ScElement {
  static readonly selector = 'sc-arcade';

  /** The session's state: the game pauses as it leaves `working`. */
  readonly status = this.useChange<SessionStatus>('idle', (status, previous) => this.tell(status, previous));

  private _current: GameId = lastGame();
  private readonly _games = new Map<GameId, ArcadeGame>();

  private readonly _refs = this.useRefs((ref) => ({
    dialog: ref(JlDialog),
    tabs: ref(JlTabs),
  }));

  templateHtml(): Node {
    return (
      <JlDialog heading="Play while it works" description={GAMES[this._current].description} size="lg">
        <JlTabs label="Games" value={this._current}>
          {GAME_ORDER.map((id) => (
            <JlTabPanel name={id} label={GAMES[id].label} icon={GAMES[id].icon}>
              <JlLoadState class={id} skeleton="cards" count={1} />
            </JlTabPanel>
          ))}
        </JlTabs>
      </JlDialog>
    );
  }

  templateCss(): string {
    return css`${BASE_STYLES}${styles}`;
  }

  onConnect(): void {
    this._refs.dialog.events.register('close', () => this._games.get(this._current)?.deactivate());
    this._refs.tabs.events.register('change', (value) => this.choose(value));
    for (const id of GAME_ORDER) {
      this.stateOf(id).events.register('retry', () => void this.load(id));
    }
  }

  open(): void {
    this._refs.dialog.show();
    this.showCurrent();
  }

  private stateOf(id: GameId): JlLoadState {
    return this.getElement<JlLoadState>(`jl-load-state.${id}`);
  }

  /** The game of the open tab runs, loaded first if it never was. */
  private showCurrent(): void {
    const game = this._games.get(this._current);
    if (!game) {
      void this.load(this._current);
      return;
    }
    game.activate();
  }

  private choose(value: string): void {
    if (!isGameId(value) || value === this._current) {
      return;
    }
    this._games.get(this._current)?.deactivate();
    this._current = value;
    keepGame(value);
    this._refs.dialog.description.set(GAMES[value].description);
    this.showCurrent();
  }

  private load(id: GameId): Promise<boolean> {
    const state = this.stateOf(id);
    return state.load({
      draw: (Game) => {
        const game = new Game();
        game.addEventListener('leave', () => this._refs.dialog.close());
        this._games.set(id, game);
        state.replaceChildren(game);
        this.activateLoaded(id, game);
      },
      fetch: GAMES[id].load,
    });
  }

  /** A game that loaded while its tab is still the one open, in the open dialog, starts there. */
  private activateLoaded(id: GameId, game: ArcadeGame): void {
    if (id !== this._current || !this._refs.dialog.open.get()) {
      return;
    }
    game.activate();
  }

  private tell(status: SessionStatus, previous: SessionStatus): void {
    const notice = NOTICES[status];
    if (!notice || status === previous || !this._refs.dialog.open.get()) {
      return;
    }
    this._games.get(this._current)?.notify(notice);
  }
}

ScArcade.register();
