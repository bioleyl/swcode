import { css } from '@jadis/core';
import { JlButton, JlIcon } from '@syware/jlib';

import { ScElement } from '../base/sc-element';
import { BASE_STYLES } from '../styles/shared';
import styles from './sc-question.css?inline';

import type { Question } from '../../../shared/protocol/session.schema';

const PICKED = 'aria-pressed';
const ANSWER_SEPARATOR = ', ';

// What the person gave each question so far: the answers picked, and their own words.
interface Reply {
  picked: string[];
  own: string;
}

const EMPTY_REPLY: Reply = { own: '', picked: [] };

/** A question's answer: the person's own words when they wrote some, else what they picked. */
const answerOf = (reply: Reply): string => reply.own.trim() || reply.picked.join(ANSWER_SEPARATOR);

const optionNode = (option: Question['options'][number]): Node => (
  <button type="button" class="option" aria-pressed="false" data-answer={option.label}>
    <span class="option-name">{option.label}</span>
    <span class="option-text">{option.description}</span>
  </button>
);

const questionNode = (question: Question, index: number): Node => (
  <fieldset class="question" data-index={String(index)}>
    <legend>
      <span class="chip">{question.header}</span>
      <span class="ask">{question.question}</span>
    </legend>
    <div class="options">{question.options.map(optionNode)}</div>
    <input class="own" type="text" placeholder="Or write your own answer" aria-label="Your own answer" />
  </fieldset>
);

/**
 * The agent's questions to the person, with answers to pick or their own to write. Inline in the conversation on
 * a computer, a sheet at the bottom of the screen on a phone. One question with one answer to pick is answered by
 * the pick; otherwise the Answer button sends them all.
 */
export class ScQuestion extends ScElement {
  static readonly selector = 'sc-question';

  readonly events = this.useEvents<{ answer: string[] }>();

  readonly questions = this.useChange<Question[]>([], (questions) => {
    this._replies.set(questions.map(() => EMPTY_REPLY));
    this._refs.list.replaceChildren(...questions.map(questionNode));
    this._refs.send.hidden = this.answersOnPick();
  });
  readonly where = this.useChange('', (value) => {
    this._refs.where.textContent = value;
  });

  private readonly _replies = this.useChange<Reply[]>([], () => this.renderSend());

  private readonly _refs = this.useRefs((ref) => ({
    list: ref('.questions'),
    send: ref<JlButton>('.send'),
    where: ref('.where'),
  }));

  templateHtml(): Node {
    return (
      <section class="card" aria-label="The agent's questions">
        <span class="grabber" aria-hidden="true" />
        <div class="head">
          <span class="badge" aria-hidden="true">
            <JlIcon name="circle-question" size={20} />
          </span>
          <div>
            <h2>The agent has a question</h2>
            <p class="where" />
          </div>
        </div>
        <div class="questions" />
        <JlButton class="send" label="Answer" variant="primary" size="lg" block disabled />
      </section>
    );
  }

  templateCss(): string {
    return css`${BASE_STYLES}${styles}`;
  }

  onConnect(): void {
    this.on(this._refs.list, 'click', (event) => this.pick(event.target));
    this.on(this._refs.list, 'input', (event) => this.write(event.target));
    this._refs.send.events.register('press', () => this.send());
  }

  /** One question with a single answer to pick: the pick is the answer. */
  private answersOnPick(): boolean {
    const questions = this.questions.get();
    return questions.length === 1 && questions[0]?.multiSelect === false;
  }

  private pick(target: EventTarget | null): void {
    const option = target instanceof Element ? target.closest<HTMLButtonElement>('.option') : null;
    const index = Number(option?.closest<HTMLElement>('.question')?.dataset.index);
    const answer = option?.dataset.answer;
    const question = this.questions.get()[index];
    if (!option || answer === undefined || !question) {
      return;
    }
    if (this.answersOnPick()) {
      this.events.emit('answer', [answer]);
      return;
    }
    const picked = this.repliesOf(index).picked;
    const chosen = question.multiSelect ? toggled(picked, answer) : [answer];
    this.updateReply(index, { picked: chosen });
    for (const each of option.parentElement?.querySelectorAll<HTMLButtonElement>('.option') ?? []) {
      each.setAttribute(PICKED, String(chosen.includes(each.dataset.answer ?? '')));
    }
  }

  private write(target: EventTarget | null): void {
    const input = target instanceof HTMLInputElement ? target : null;
    const index = Number(input?.closest<HTMLElement>('.question')?.dataset.index);
    if (!input || Number.isNaN(index)) {
      return;
    }
    this.updateReply(index, { own: input.value });
    this._refs.send.hidden = this.answersOnPick() && input.value.trim() === '';
  }

  private repliesOf(index: number): Reply {
    return this._replies.get()[index] ?? EMPTY_REPLY;
  }

  private updateReply(index: number, change: Partial<Reply>): void {
    this._replies.set((replies) =>
      replies.map((reply, each) => (each === index ? { ...reply, ...change } : reply))
    );
  }

  private renderSend(): void {
    const replies = this._replies.get();
    this._refs.send.disabled.set(replies.length === 0 || replies.some((reply) => answerOf(reply) === ''));
  }

  private send(): void {
    this.events.emit('answer', this._replies.get().map(answerOf));
  }
}

const toggled = (picked: readonly string[], answer: string): string[] =>
  picked.includes(answer) ? picked.filter((each) => each !== answer) : [...picked, answer];

ScQuestion.register();
