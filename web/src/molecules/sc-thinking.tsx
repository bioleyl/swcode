import { css } from '@jadis/core';
import { JlIcon } from '@syware/jlib';

import { ScElement } from '../base/sc-element';
import { keepingEnd } from '../lib/follow-end';
import { BASE_STYLES } from '../styles/shared';
import styles from './sc-thinking.css?inline';

const STREAMING_LABEL = 'Thinking…';
const DONE_LABEL = 'Thought';
const OPEN = 'open';
// Less than a line from the end counts as following along: scrolling up even a little stops it.
const FOLLOW_DISTANCE_PX = 16;

/**
 * What the model thought before it answered: open while it streams, folded once the answer comes. While it
 * streams, a long thought keeps its newest lines in view, unless people scrolled up to read.
 */
export class ScThinking extends ScElement {
  static readonly selector = 'sc-thinking';

  readonly text = this.useChange('', (value) => {
    keepingEnd(this._refs.body, FOLLOW_DISTANCE_PX, () => {
      this._refs.body.textContent = value;
    });
  });
  readonly done = this.useChange(
    false,
    (value) => {
      this._refs.label.textContent = value ? DONE_LABEL : STREAMING_LABEL;
      this.reflect('done', value);
      this.toggle(!value);
    },
    { immediate: true }
  );

  private readonly _refs = this.useRefs((ref) => ({
    body: ref('.body'),
    head: ref<HTMLButtonElement>('button'),
    label: ref('.text'),
  }));

  append(text: string): void {
    this.text.set((sofar) => `${sofar}${text}`);
  }

  /** The whole reasoning, as the reply recorded it, replaces what streamed. */
  finish(text: string): void {
    this.text.set(text);
    this.done.set(true);
  }

  templateHtml(): Node {
    return (
      <>
        <button type="button" aria-expanded="true">
          <span class="text" />
          <JlIcon name="chevron-down" size={14} />
        </button>
        <p class="body" />
      </>
    );
  }

  templateCss(): string {
    return css`${BASE_STYLES}${styles}`;
  }

  onConnect(): void {
    this.on(this._refs.head, 'click', () => this.toggle(!this.hasAttribute(OPEN)));
  }

  private toggle(open: boolean): void {
    this.reflect(OPEN, open);
    this._refs.head.setAttribute('aria-expanded', String(open));
  }
}

ScThinking.register();
