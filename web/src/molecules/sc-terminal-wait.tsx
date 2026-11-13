import { css } from '@jadis/core';
import { JlButton, JlIcon } from '@syware/jlib';

import { ScElement } from '../base/sc-element';
import { BASE_STYLES } from '../styles/shared';
import styles from './sc-terminal-wait.css?inline';

const WHAT_TO_DO =
  'If it asks for something (a password for sudo or ssh, a yes or a no), type it in that terminal: swcode never sees what you type there. The agent goes on once the command ends.';

/**
 * A command of the agent runs in a terminal of the app, and its call waits for it to end: where it runs, what to do
 * there, and the way to the terminal. It goes with the call's result.
 */
export class ScTerminalWait extends ScElement {
  static readonly selector = 'sc-terminal-wait';

  readonly events = this.useEvents<{ open: undefined }>();

  /** The terminal's name, as the side panel lists it. */
  readonly terminal = this.useChange('', (title) => {
    this._refs.heading.textContent = `Runs in the terminal “${title}”`;
  });

  private readonly _refs = this.useRefs((ref) => ({
    heading: ref('h2'),
    open: ref(JlButton),
  }));

  templateHtml(): Node {
    return (
      <section class="card" aria-label="A command runs in a terminal, where it may ask you something">
        <div class="head">
          <span class="badge" aria-hidden="true">
            <JlIcon name="terminal" size={20} />
          </span>
          <div>
            <h2 />
            <p class="what">{WHAT_TO_DO}</p>
          </div>
        </div>
        <JlButton label="Open the terminal" variant="primary" />
      </section>
    );
  }

  templateCss(): string {
    return css`${BASE_STYLES}${styles}`;
  }

  onConnect(): void {
    this._refs.open.events.register('press', () => this.events.emit('open', undefined));
  }
}

ScTerminalWait.register();
