import { css } from '@jadis/core';
import { JlButton } from '@syware/jlib';

import { ScElement } from '../base/sc-element';
import { BASE_STYLES } from '../styles/shared';
import styles from './sc-game-screen.css?inline';

import type { GameNotice, GameScreen } from '../types/game.types';

/** The screen of a game paused because the session wants the person: what it wants, and the way back to it. */
export const noticeScreen = (notice: GameNotice): GameScreen => ({
  action: 'Keep playing',
  heading: notice.heading,
  leave: true,
  text: notice.text,
});

/**
 * What covers a game that does not run: its title, a pause, its end, or what the session wants. Its button starts
 * or resumes the game (`action`); with a notice, another leads back to the session (`leave`).
 */
export class ScGameScreen extends ScElement {
  static readonly selector = 'sc-game-screen';

  readonly events = this.useEvents<{ action: undefined; leave: undefined }>();

  private readonly _refs = this.useRefs((ref) => ({
    action: ref<JlButton>('.action'),
    heading: ref('.heading'),
    leave: ref<JlButton>('.leave'),
    text: ref('.text'),
  }));

  templateHtml(): Node {
    return (
      <div class="screen" role="status">
        <h2 class="heading" />
        <p class="text" />
        <div class="actions">
          <JlButton class="action" variant="primary" />
          <JlButton class="leave" label="Back to the session" variant="ghost" hidden />
        </div>
      </div>
    );
  }

  templateCss(): string {
    return css`${BASE_STYLES}${styles}`;
  }

  onConnect(): void {
    this._refs.action.events.register('press', () => this.events.emit('action'));
    this._refs.leave.events.register('press', () => this.events.emit('leave'));
  }

  /** Shows a screen over the game, or none while it runs. */
  show(screen: GameScreen | undefined): void {
    this.hidden = screen === undefined;
    if (!screen) {
      return;
    }
    this._refs.heading.textContent = screen.heading;
    this._refs.text.textContent = screen.text;
    this._refs.action.label.set(screen.action);
    this._refs.leave.hidden = !screen.leave;
  }
}

ScGameScreen.register();
