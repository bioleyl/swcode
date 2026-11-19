import { createElement, css } from '@jadis/core';
import { JlButton, JlCard, JlDialog, JlField, JlFlow, JlSelect } from '@syware/jlib';

import { ScElement } from '../base/sc-element';
import { decisionMapOf } from '../lib/decision-map';
import { BASE_STYLES } from '../styles/shared';
import styles from './sc-decision-map.css?inline';

import type { SessionEvent } from '../../../shared/protocol/session.schema';
import type {
  Decision,
  DecisionCall,
  DecisionMap,
  DecisionQuestion,
  DecisionRecord,
} from '../types/decision-map.types';

const EMPTY_MAP: DecisionMap = { decisions: [], graph: { edges: [], nodes: [] } };
const CHOOSE_TEXT = 'Select a decision in the map to read what was decided.';
const EMPTY_TEXT = 'No decision is in this part of the session yet.';
const WHEN_FORMAT: Intl.DateTimeFormatOptions = { dateStyle: 'medium', timeStyle: 'short' };

const hintText = (count: number): string =>
  count === 1
    ? '1 decision · select it to read what was decided.'
    : `${count} decisions · select one to read what was decided.`;

const whenText = (iso: string): string => new Date(iso).toLocaleString(undefined, WHEN_FORMAT);

/** A call the model made, in a line: its tool, what it did, and how it ended. */
const callNode = (call: DecisionCall): Node => (
  <div class="call" data-ok={String(call.ok ?? 'unknown')}>
    <span class="call-name">{call.name}</span>
    <span class="call-summary">{call.summary}</span>
    {call.meta === undefined ? '' : <span class="call-meta">{call.meta}</span>}
  </div>
);

/** What the new turn says when the person answers the agent's questions again. */
const changedAnswersText = (
  questions: readonly DecisionQuestion[],
  areas: readonly HTMLTextAreaElement[]
): string => {
  const lines = ['Change my answers to your questions:', ''];
  for (const [index, question] of questions.entries()) {
    lines.push(`Q: ${question.question}`, `My new answer: ${areas[index]?.value.trim() ?? ''}`, '');
  }
  lines.push('Continue from there.');
  return lines.join('\n');
};

/** What the new turn says when the person chooses another option than the model recorded. */
const changedChoiceText = (record: DecisionRecord, chosen: string): string =>
  `Change the decision “${record.question}” to “${chosen}” (it was “${record.chosen}”). Continue from there.`;

/** Whether the person can change this decision: an answer they gave, or a choice the model recorded. */
const canChange = (decision: Decision): boolean =>
  (decision.questions?.length ?? 0) > 0 || decision.record !== undefined;

/**
 * The decisions of a session as a graph: each a row below the one before, from the person's message and the model's
 * replies to the questions and permissions it waited on. Selecting one tells what it decided, and the calls it made;
 * an answer or a recorded choice can be changed, which starts a new session from that decision.
 */
export class ScDecisionMap extends ScElement {
  static readonly selector = 'sc-decision-map';

  readonly events = this.useEvents<{ change: { keep: number; text: string } }>();

  readonly history = this.useChange<readonly SessionEvent[]>([], (events) => this.draw(events));

  private _map: DecisionMap = EMPTY_MAP;
  private _selected = '';
  // The fields of the change dialog, by what they edit: one area a question, or the options of a recorded choice.
  private _areas: HTMLTextAreaElement[] = [];
  private _choice: JlSelect<string> | undefined;

  private readonly _refs = this.useRefs((ref) => ({
    calls: ref('.calls'),
    cancel: ref<JlButton>('.cancel'),
    card: ref(JlCard),
    change: ref<JlButton>('.change'),
    choose: ref('.choose'),
    dialog: ref(JlDialog),
    fields: ref('.fields'),
    flow: ref(JlFlow),
    hint: ref('.hint'),
    save: ref<JlButton>('.save'),
    text: ref('.text'),
    when: ref('.when'),
  }));

  templateHtml(): Node {
    return (
      <>
        <div class="map">
          <p class="hint" />
          <div class="body">
            <div class="flow">
              <JlFlow readonly label="The decisions of this session" />
            </div>
            <div class="read">
              <p class="choose" />
              <JlCard class="detail" heading="Decision" hidden>
                <JlButton slot="actions" class="change" variant="ghost" size="sm" label="Change…" hidden />
                <p class="when" />
                <pre class="text" />
                <div class="calls" />
              </JlCard>
            </div>
          </div>
        </div>
        <JlDialog
          class="change-dialog"
          heading="Change the decision"
          description="A new session starts from here with your change, in the same folder; this one stays as it was.">
          <div class="fields" />
          <JlButton slot="actions" class="cancel" label="Cancel" />
          <JlButton slot="actions" class="save" variant="primary" label="Change and replay" />
        </JlDialog>
      </>
    );
  }

  templateCss(): string {
    return css`${BASE_STYLES}${styles}`;
  }

  onConnect(): void {
    this._refs.flow.events.register('select', (id) => this.select(id));
    this._refs.change.events.register('press', () => this.openChange());
    this._refs.cancel.events.register('press', () => this._refs.dialog.close());
    this._refs.save.events.register('press', () => this.saveChange());
  }

  /** Shows the last decision, where the session stands now: the page calls it once the map is in sight. */
  showLatest(): void {
    const last = this._map.decisions.at(-1);
    if (last === undefined) {
      return;
    }
    this.select(last.id);
    // The card opens below the map and the map's height changes with it: the view moves once the browser laid it out.
    requestAnimationFrame(() => {
      this._refs.flow.selected.set(last.id);
      this._refs.flow.reveal(last.id);
    });
  }

  private select(id: string): void {
    this._selected = id;
    this.drawDetail();
  }

  private draw(events: readonly SessionEvent[]): void {
    this._map = decisionMapOf(events);
    this._refs.hint.textContent = hintText(this._map.decisions.length);
    this._refs.flow.graph.set(this._map.graph);
    if (!this._map.decisions.some((decision) => decision.id === this._selected)) {
      this._selected = '';
    }
    this.drawDetail();
  }

  private drawDetail(): void {
    const decision = this.decision();
    this._refs.choose.textContent = this._map.decisions.length === 0 ? EMPTY_TEXT : CHOOSE_TEXT;
    this._refs.choose.hidden = decision !== undefined;
    this._refs.card.hidden = decision === undefined;
    this._refs.change.hidden = decision === undefined || !canChange(decision);
    if (decision === undefined) {
      return;
    }
    this._refs.card.heading.set(decision.heading);
    this._refs.when.textContent = whenText(decision.at);
    this._refs.text.textContent = decision.text;
    this._refs.text.hidden = decision.text === '';
    this._refs.calls.replaceChildren(...decision.calls.map(callNode));
    this._refs.calls.hidden = decision.calls.length === 0;
  }

  /** The decision the detail card shows now, when there is one. */
  private decision(): Decision | undefined {
    return this._map.decisions.find((candidate) => candidate.id === this._selected);
  }

  /** The change dialog: an area per question the agent asked, or the options of the choice it recorded. */
  private openChange(): void {
    const decision = this.decision();
    if (decision === undefined || !canChange(decision)) {
      return;
    }
    this._areas = [];
    this._choice = undefined;
    this._refs.dialog.heading.set(decision.questions === undefined ? 'Change the decision' : 'Change the answer');
    this._refs.fields.replaceChildren();
    if (decision.questions !== undefined) {
      for (const question of decision.questions) {
        const area = document.createElement('textarea');
        area.className = 'answer';
        area.value = question.answer;
        this._areas.push(area);
        const field = createElement(JlField, { props: { label: question.question } });
        field.append(area);
        this._refs.fields.append(field);
      }
    }
    if (decision.record !== undefined) {
      const { chosen, options, question } = decision.record;
      const choice = createElement(JlSelect, { props: { label: question } });
      choice.options.set(options.map((option) => ({ label: option, value: option })));
      choice.value.set(chosen);
      this._choice = choice;
      const field = createElement(JlField, { props: { label: 'Choose another option' } });
      field.append(choice);
      this._refs.fields.append(field);
    }
    this._refs.dialog.show();
  }

  private saveChange(): void {
    const decision = this.decision();
    if (decision === undefined) {
      return;
    }
    const text =
      decision.questions !== undefined
        ? changedAnswersText(decision.questions, this._areas)
        : decision.record === undefined
          ? ''
          : changedChoiceText(decision.record, this._choice?.value.get() ?? decision.record.chosen);
    if (text === '') {
      return;
    }
    this.events.emit('change', { keep: decision.cut, text });
    this._refs.dialog.close();
  }
}

ScDecisionMap.register();
