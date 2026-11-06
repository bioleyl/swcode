import { css } from '@jadis/core';
import { JlButton, JlIcon } from '@syware/jlib';

import { ScElement } from '../base/sc-element';
import { ScMessage } from '../molecules/sc-message';
import { BASE_STYLES } from '../styles/shared';
import styles from './sc-peek.css?inline';

import type { PeekState } from '../../../shared/protocol/messages.schema';

const ICON_SIZE = 14;
const ASKING = 'Asking…';
// What the card says by state; a failure says its own reason.
const STATE_LINES: Readonly<Record<PeekState, string>> = {
  answering: 'Answering…',
  done: '',
  failed: 'No answer came.',
  waiting: 'Waiting for the agent to finish its request…',
};

/**
 * A question asked beside the session (/peek) and its answer as it comes, above the composer: never part of the
 * conversation, gone once closed or the page left. A new question takes the place of the last one.
 */
export class ScPeek extends ScElement {
  static readonly selector = 'sc-peek';

  // The question shown: the answers of another one are not this card's any more.
  private _peekId = '';
  private _answer = '';

  private readonly _refs = this.useRefs((ref) => ({
    answer: ref(ScMessage),
    close: ref<JlButton>('.close'),
    question: ref('.question'),
    state: ref('.state'),
  }));

  templateHtml(): Node {
    return (
      <div class="card">
        <div class="head">
          <JlIcon name="eye" size={ICON_SIZE} />
          <span class="title">Peek</span>
          <span class="question" />
          <JlButton class="close" icon="x" hint="Close" variant="ghost" size="sm" />
        </div>
        <ScMessage />
        <p class="state" role="status" />
      </div>
    );
  }

  templateCss(): string {
    return css`${BASE_STYLES}${styles}`;
  }

  onConnect(): void {
    this._refs.close.events.register('press', () => this.close());
  }

  /** A new question, shown until its answer comes. */
  ask(peekId: string, question: string): void {
    this._peekId = peekId;
    this._answer = '';
    this._refs.question.textContent = question;
    this._refs.question.title = question;
    this._refs.answer.text.set('');
    this.renderState('', ASKING);
    this.hidden = false;
  }

  /** Where the question shown is: waiting, answering, answered, or why it failed. */
  showState(peekId: string, state: PeekState, message: string): void {
    if (peekId !== this._peekId) {
      return;
    }
    this.renderState(state, message || STATE_LINES[state]);
  }

  /** A piece of the answer to the question shown. */
  delta(peekId: string, text: string): void {
    if (peekId !== this._peekId) {
      return;
    }
    this._answer += text;
    this._refs.answer.text.set(this._answer);
  }

  private renderState(state: PeekState | '', line: string): void {
    this._refs.state.textContent = line;
    this._refs.state.dataset.state = state;
  }

  private close(): void {
    this._peekId = '';
    this.hidden = true;
  }
}

ScPeek.register();
