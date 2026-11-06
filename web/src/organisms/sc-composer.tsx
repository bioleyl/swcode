import { createElement, css } from '@jadis/core';
import { JlButton, JlIcon, JlPopover } from '@syware/jlib';

import { MAX_PICTURES } from '../../../shared/protocol/picture.schema';
import { ScElement } from '../base/sc-element';
import { MODE_LABELS } from '../lib/mode-labels';
import { peekQuestion } from '../lib/peek';
import { isPicture, pictureUrl, shrinkPicture } from '../lib/pictures';
import { presetOf } from '../lib/presets';
import { THINKING_LABELS } from '../lib/thinking-labels';
import { ScSessionChoices } from '../molecules/sc-session-choices';
import { presetChoices, presetsBus } from '../services/presets';
import { BASE_STYLES } from '../styles/shared';
import styles from './sc-composer.css?inline';

import type { PictureUpload } from '../../../shared/protocol/picture.schema';
import type { PermissionMode, SessionKind, ThinkingMode } from '../../../shared/protocol/session.schema';
import type { PromptDraft } from '../types/composer.types';

const ENTER = 'Enter';
/** What the empty field asks for: a chat's question, or a coding session's change. */
const IDLE_PLACEHOLDERS: Readonly<Record<SessionKind, string>> = {
  chat: 'Ask anything',
  code: 'Ask swcode, or describe the change you want',
};
const QUEUE_PLACEHOLDER = 'Add a message for when this turn ends';
const UNSEEN_NOTE = 'This model cannot see pictures: it is only told they are here.';
const UNREADABLE_PICTURE = 'That picture could not be read: paste a PNG, a JPEG or a screenshot.';
const CHOICES_ICON_SIZE = 14;
const SUMMARY_SEPARATOR = ' · ';
const CONCISE_LABEL = 'Token economy';

/**
 * Where the person writes: the message and the pictures pasted or dropped with it, and send, or stop while the agent
 * works. One chip says what the session works with (its preset, its permissions, how much its model thinks, token
 * economy) and opens them all in a panel; a chat has no permissions, since it runs nothing. A picture is made small as
 * it comes in.
 */
export class ScComposer extends ScElement {
  static readonly selector = 'sc-composer';

  readonly events = this.useEvents<{
    send: PromptDraft;
    /** A question beside the session: the composer's text after /peek. */
    peek: string;
    stop: undefined;
    play: undefined;
    preset: string;
    mode: PermissionMode;
    thinking: ThinkingMode;
    concise: boolean;
  }>();

  // While a turn runs, a message waits for the next one: send stays, stop comes beside it, and a game to pass the time.
  /** Whether there is a session to peek at: /peek beside a session only; elsewhere it is words. */
  peeks = false;
  readonly working = this.useChange(false, (value) => {
    this._refs.play.hidden = !value;
    this.renderStop();
    this.renderPlaceholder();
  });
  /** Subagents at work in the background: stop shows, to stop them, while a message still starts a turn at once. */
  readonly background = this.useChange(false, () => this.renderStop());
  /** A chat or a coding session: what the field asks for, and whether the session has permissions to choose. */
  readonly kind = this.useChange<SessionKind>(
    'code',
    () => {
      this.renderPlaceholder();
      this.renderChoices();
    },
    { immediate: true }
  );
  readonly mode = this.useChange<PermissionMode>('auto', () => this.renderChoices(), { immediate: true });
  readonly thinking = this.useChange<ThinkingMode>('full', () => this.renderChoices(), { immediate: true });
  /** The session's preset: the default one when empty, or when it was removed. */
  readonly preset = this.useChange('', () => this.renderChoices(), { immediate: true });
  /** Token economy: each message sent asks the model to be concise. */
  readonly concise = this.useChange(false, () => this.renderChoices(), { immediate: true });
  /** Whether the model sees pictures: without vision, the pictures still go, and the model is told of them. */
  readonly seesPictures = this.useChange(true, () => this.renderPictures());
  /** How full the context is, in words: "24.1k / 64k". */
  readonly context = this.useChange('', (value) => {
    this._refs.context.textContent = value;
  });

  // The pictures waiting to go with the message, made small already.
  private _pictures: PictureUpload[] = [];

  private readonly _refs = this.useRefs((ref) => ({
    box: ref<HTMLFormElement>('form'),
    choices: ref(ScSessionChoices),
    choicesChip: ref<HTMLButtonElement>('.choices'),
    context: ref('.context'),
    pictureNote: ref('.picture-note'),
    pictures: ref('.pictures'),
    play: ref<JlButton>('.play'),
    send: ref<JlButton>('.send'),
    stop: ref<JlButton>('.stop'),
    summary: ref('.summary'),
    text: ref<HTMLTextAreaElement>('textarea'),
    thumbs: ref('.thumbs'),
  }));

  focusText(): void {
    this._refs.text.focus();
  }

  templateHtml(): Node {
    return (
      <form class="box">
        <div class="pictures" hidden>
          <div class="thumbs" />
          <p class="picture-note" role="status" />
        </div>
        <label class="sr-only" htmlFor="prompt">
          Message swcode
        </label>
        <textarea id="prompt" rows={2} />
        <div class="bar">
          <JlPopover placement="top-start" heading="This session">
            <button slot="trigger" type="button" class="chip choices">
              <JlIcon name="sliders-horizontal" size={CHOICES_ICON_SIZE} />
              <span class="summary" />
              <JlIcon class="chevron" name="chevron-down" size={CHOICES_ICON_SIZE} />
            </button>
            <ScSessionChoices />
          </JlPopover>
          <span class="context" />
          <JlButton class="play" icon="gamepad" hint="Play while it works" variant="ghost" hidden />
          <JlButton class="stop" icon="stop" hint="Stop" hidden />
          <JlButton class="send" variant="primary" icon="arrow-up" hint="Send" />
        </div>
      </form>
    );
  }

  templateCss(): string {
    return css`${BASE_STYLES}${styles}`;
  }

  onConnect(): void {
    this._refs.send.events.register('press', () => this.submit());
    this._refs.stop.events.register('press', () => this.events.emit('stop'));
    this._refs.play.events.register('press', () => this.events.emit('play'));
    this.on(this._refs.text, 'keydown', (event) => this.keyDown(event));
    this.on(this._refs.text, 'paste', (event) => this.take(event, event.clipboardData));
    this.on(this._refs.box, 'dragover', (event) => event.preventDefault());
    this.on(this._refs.box, 'drop', (event) => this.take(event, event.dataTransfer));
    // A choice picked in the panel: shown at once, and said to the page, which tells the daemon.
    this._refs.choices.events.register('preset', (preset) => {
      this.preset.set(preset);
      this.events.emit('preset', preset);
    });
    this._refs.choices.events.register('mode', (mode) => {
      this.mode.set(mode);
      this.events.emit('mode', mode);
    });
    this._refs.choices.events.register('thinking', (thinking) => {
      this.thinking.set(thinking);
      this.events.emit('thinking', thinking);
    });
    this._refs.choices.events.register('concise', (concise) => {
      this.concise.set(concise);
      this.events.emit('concise', concise);
    });
    this.onBus(presetsBus, 'changed', () => this.renderChoices());
    this.renderChoices();
  }

  private keyDown(event: KeyboardEvent): void {
    // Enter sends, Shift+Enter starts a new line; an input method still composing keeps its Enter.
    if (event.key !== ENTER || event.shiftKey || event.isComposing) {
      return;
    }
    event.preventDefault();
    this.submit();
  }

  /** Pictures pasted or dropped: they join the message; anything else is pasted as usual. */
  private take(event: Event, transfer: DataTransfer | null): void {
    const files = [...(transfer?.files ?? [])].filter(isPicture);
    if (files.length === 0) {
      return;
    }
    event.preventDefault();
    void this.addPictures(files);
  }

  private async addPictures(files: readonly File[]): Promise<void> {
    for (const file of files.slice(0, MAX_PICTURES - this._pictures.length)) {
      const picture = await shrinkPicture(file).catch(() => undefined);
      if (!picture) {
        this.renderPictures(UNREADABLE_PICTURE);
        continue;
      }
      this._pictures = [...this._pictures, picture];
      this.renderPictures();
    }
  }

  private removePicture(index: number): void {
    this._pictures = this._pictures.filter((_, each) => each !== index);
    this.renderPictures();
    this._refs.text.focus();
  }

  /** A button keeps its clicks to itself: each picture's own says which one goes. */
  private thumbNode(picture: PictureUpload, index: number): Node {
    const remove = createElement(JlButton, {
      attrs: { class: 'remove' },
      props: { hint: 'Remove this picture', icon: 'x', size: 'sm', variant: 'ghost' },
    });
    remove.events.register('press', () => this.removePicture(index));
    return (
      <span class="thumb">
        <img src={pictureUrl(picture)} alt={`Pasted, ${index + 1} of ${this._pictures.length}`} />
        {remove}
      </span>
    );
  }

  /** The waiting pictures, and a word under them: one could not be read, or the model will not see them. */
  private renderPictures(problem = ''): void {
    const unseen = this.seesPictures.get() || this._pictures.length === 0 ? '' : UNSEEN_NOTE;
    const note = problem || unseen;
    this._refs.pictures.hidden = this._pictures.length === 0 && note === '';
    this._refs.thumbs.replaceChildren(...this._pictures.map((picture, index) => this.thumbNode(picture, index)));
    this._refs.pictureNote.textContent = note;
  }

  private submit(): void {
    const text = this._refs.text.value.trim();
    const question = this.peeks ? peekQuestion(text) : undefined;
    if (question !== undefined) {
      this.peek(question);
      return;
    }
    if (text === '' && this._pictures.length === 0) {
      return;
    }
    const draft = { pictures: this._pictures, text };
    this._refs.text.value = '';
    this._pictures = [];
    this.renderPictures();
    this.events.emit('send', draft);
  }

  /** A question beside the session goes, the pictures waiting stay for the next message. */
  private peek(question: string): void {
    if (question === '') {
      return;
    }
    this._refs.text.value = '';
    this.events.emit('peek', question);
  }

  private renderStop(): void {
    this._refs.stop.hidden = !this.working.get() && !this.background.get();
  }

  private renderPlaceholder(): void {
    this._refs.text.placeholder = this.working.get() ? QUEUE_PLACEHOLDER : IDLE_PLACEHOLDERS[this.kind.get()];
  }

  /** The chip's words, the panel's choices: the preset shown is the one the daemon uses, the default when unknown. */
  private renderChoices(): void {
    const choices = presetChoices();
    const preset = presetOf(choices, this.preset.get());
    const code = this.kind.get() === 'code';
    const summary = [
      preset?.name,
      code ? MODE_LABELS[this.mode.get()] : undefined,
      THINKING_LABELS[this.thinking.get()],
      this.concise.get() ? CONCISE_LABEL : undefined,
    ]
      .filter((part) => part !== undefined)
      .join(SUMMARY_SEPARATOR);
    this._refs.summary.textContent = summary;
    // A narrow composer cuts the words short: the tooltip keeps them whole.
    this._refs.choicesChip.title = summary;
    const panel = this._refs.choices;
    panel.choices.set(choices);
    panel.preset.set(preset?.id ?? '');
    panel.permissions.set(code);
    panel.mode.set(this.mode.get());
    panel.thinking.set(this.thinking.get());
    panel.concise.set(this.concise.get());
  }
}

ScComposer.register();
