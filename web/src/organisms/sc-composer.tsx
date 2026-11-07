import { createElement, css } from '@jadis/core';
import { JlButton, JlIcon, JlKbd, JlPopover } from '@syware/jlib';

import { MAX_PICTURES } from '../../../shared/protocol/picture.schema';
import { ScElement } from '../base/sc-element';
import {
  COMMAND_PREFIX,
  COMMANDS,
  commandLine,
  commandText,
  menuMatching,
  skillLine,
  skillText,
  stepIn,
} from '../lib/commands';
import { MODE_LABELS } from '../lib/mode-labels';
import { isPicture, pictureUrl, shrinkPicture } from '../lib/pictures';
import { presetOf } from '../lib/presets';
import { THINKING_LABELS } from '../lib/thinking-labels';
import { ScCommandHelp } from '../molecules/sc-command-help';
import { ScSessionChoices } from '../molecules/sc-session-choices';
import { presetChoices, presetsBus } from '../services/presets';
import { BASE_STYLES } from '../styles/shared';
import styles from './sc-composer.css?inline';

import type { PictureUpload } from '../../../shared/protocol/picture.schema';
import type { PermissionMode, SessionKind, ThinkingMode } from '../../../shared/protocol/session.schema';
import type { SkillSummary } from '../../../shared/protocol/skill.schema';
import type { CommandName } from '../lib/commands';
import type { CommandLine, MenuEntry } from '../types/commands.types';
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
const COMMAND_ICON_SIZE = 16;
const SKILL_ICON = 'book-open';
// The list's id and each option's, for the field to say which one is chosen (`aria-activedescendant`).
const COMMAND_LIST_ID = 'command-list';
const OPTION_ID_PREFIX = 'command-';
const SKILL_OPTION_ID_PREFIX = 'skill-';

const optionId = (entry: MenuEntry): string =>
  entry.kind === 'command' ? `${OPTION_ID_PREFIX}${entry.name}` : `${SKILL_OPTION_ID_PREFIX}${entry.skill.name}`;

/**
 * Where the person writes: the message and the pictures pasted or dropped with it, and send, or stop while the agent
 * works. One chip says what the session works with (its preset, its permissions, how much its model thinks, token
 * economy) and opens them all in a panel; a chat has no permissions, since it runs nothing. A picture is made small as
 * it comes in.
 *
 * A message that starts with a slash lists the commands available here, then the skills, narrowed as the name is
 * typed: the arrows move through them, Enter runs the one chosen (or completes one that takes words), Tab completes
 * it, Escape closes the list. A skill runs as a message, `/<its name>` and the words after it, which the daemon reads
 * the skill for. The list is drawn in the field's own shadow root, for the field to name the option chosen.
 */
export class ScComposer extends ScElement {
  static readonly selector = 'sc-composer';

  readonly events = this.useEvents<{
    send: PromptDraft;
    /** A question beside the session: the composer's text after /peek. */
    peek: string;
    /** /new: a new session with this one's settings. */
    newSession: undefined;
    /** /compact: the conversation summarized now. */
    compact: undefined;
    stop: undefined;
    play: undefined;
    preset: string;
    mode: PermissionMode;
    thinking: ThinkingMode;
    concise: boolean;
  }>();

  // While a turn runs, a message waits for the next one: send stays, stop comes beside it, and a game to pass the
  // time. `working` counts the subagents at work too, the way the session's own status does.
  /** The commands available here: none where there is no session yet, where a slash is only a word. */
  readonly commands = this.useChange<CommandName[]>([], (names) => {
    this._refs.help.commands.set(names);
    this.renderMenu();
  });
  /** The skills here, which run as messages: those the project has on. */
  readonly skills = this.useChange<SkillSummary[]>([], (skills) => {
    this._refs.help.skills.set(skills);
    this.renderMenu();
  });
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
  // The commands and skills listed for what is typed, and the one chosen among them.
  private _matches: MenuEntry[] = [];
  private _chosen = 0;
  // The text the list was closed on (Escape, or the field left): it stays closed until the text changes.
  private _dismissed: string | undefined;

  private readonly _runs: Readonly<Record<CommandName, (argument: string) => void>> = {
    compact: () => this.emitCommand('compact'),
    help: () => this.showHelp(),
    new: () => this.emitCommand('newSession'),
    peek: (question) => this.peek(question),
  };

  // The keys of the field while the list shows, by their `key`; any other key types as usual.
  private readonly _listKeys: ReadonlyMap<string, () => void> = new Map([
    ['ArrowDown', () => this.step(1)],
    ['ArrowUp', () => this.step(-1)],
    ['Enter', () => this.choose()],
    ['Escape', () => this.dismiss()],
    ['Tab', () => this.complete(this._matches[this._chosen])],
  ]);

  private readonly _refs = this.useRefs((ref) => ({
    box: ref<HTMLFormElement>('form'),
    choices: ref(ScSessionChoices),
    choicesChip: ref<HTMLButtonElement>('.choices'),
    commandList: ref('.command-menu'),
    commandOptions: ref('.options'),
    context: ref('.context'),
    help: ref(ScCommandHelp),
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
        <div class="command-menu" hidden>
          <div class="options" role="listbox" id={COMMAND_LIST_ID} aria-label="Commands" />
          <p class="keys" aria-hidden="true">
            <JlKbd keys="up" />
            <JlKbd keys="down" /> to choose · <JlKbd keys="enter" /> to run · <JlKbd keys="tab" /> to complete ·{' '}
            <JlKbd keys="esc" /> to close
          </p>
        </div>
        <div class="pictures" hidden>
          <div class="thumbs" />
          <p class="picture-note" role="status" />
        </div>
        <label class="sr-only" htmlFor="prompt">
          Message swcode
        </label>
        <textarea id="prompt" rows={2} aria-autocomplete="list" />
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
        <ScCommandHelp />
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
    this.on(this._refs.text, 'input', () => this.typed());
    this.on(this._refs.text, 'blur', () => this.dismiss());
    this.on(this._refs.text, 'focus', () => this.reopen());
    // A press on the list keeps the field focused, so it never closes before its click.
    this.on(this._refs.commandOptions, 'mousedown', (event) => event.preventDefault());
    this.on(this._refs.commandOptions, 'click', (event) => this.optionClicked(event));
    this.on(this._refs.commandOptions, 'mousemove', (event) => this.optionHovered(event));
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
    if (event.isComposing || this.listKey(event)) {
      return;
    }
    if (event.key !== ENTER || event.shiftKey) {
      return;
    }
    event.preventDefault();
    this.submit();
  }

  /** A key the list takes while it shows: whether it took it. With a modifier, a key does what it usually does. */
  private listKey(event: KeyboardEvent): boolean {
    const action = this._listKeys.get(event.key);
    const modified = event.shiftKey || event.altKey || event.ctrlKey || event.metaKey;
    if (action === undefined || modified || this._matches.length === 0) {
      return false;
    }
    event.preventDefault();
    action();
    return true;
  }

  /** What is typed changed: the list follows it, its first command chosen. */
  private typed(): void {
    this._chosen = 0;
    this.renderMenu();
  }

  private step(step: number): void {
    this.markChosen(stepIn(this._chosen, step, this._matches.length));
    this.revealChosen();
  }

  /** Scrolls a long list, and it alone, just enough for the row chosen to show. */
  private revealChosen(): void {
    const list = this._refs.commandOptions;
    const option = list.children[this._chosen];
    if (!(option instanceof HTMLElement)) {
      return;
    }
    // The list and its rows have the menu as their offset parent: this is the row's place in the list.
    const top = option.offsetTop - list.offsetTop;
    const bottom = top + option.offsetHeight;
    if (top < list.scrollTop) {
      list.scrollTop = top;
      return;
    }
    if (bottom > list.scrollTop + list.clientHeight) {
      list.scrollTop = bottom - list.clientHeight;
    }
  }

  /** The list closes on the text as it is, and opens again once the text changes or the field is back in focus. */
  private dismiss(): void {
    this._dismissed = this._refs.text.value;
    this.renderMenu();
  }

  private reopen(): void {
    this._dismissed = undefined;
    this.renderMenu();
  }

  /** Runs the row chosen: a skill as a message alone; a command, or completes it when it takes words. */
  private choose(entry = this._matches[this._chosen]): void {
    if (entry === undefined) {
      return;
    }
    if (entry.kind === 'skill') {
      this._refs.text.value = skillLine(entry.skill.name);
      this.submit();
      return;
    }
    this.chooseCommand(entry.name);
  }

  /** Runs the command chosen, or completes it when it takes words: the person writes them next. */
  private chooseCommand(name: CommandName): void {
    if (COMMANDS[name].argument !== undefined) {
      this.complete({ kind: 'command', name });
      return;
    }
    this.run({ argument: '', name });
  }

  /** The row chosen in the field, the caret after it: a skill, a space after it for the words it works on. */
  private complete(entry: MenuEntry | undefined): void {
    if (entry === undefined) {
      return;
    }
    const text = entry.kind === 'command' ? commandText(entry.name) : skillText(entry.skill.name);
    this._refs.text.value = text;
    this._refs.text.setSelectionRange(text.length, text.length);
    this.typed();
  }

  /** The row of the option an event happened on: -1 off the options. */
  private optionIndex(event: Event): number {
    const option = event.target instanceof Element ? event.target.closest('[data-index]') : null;
    return option ? Number(option.getAttribute('data-index')) : -1;
  }

  private optionClicked(event: Event): void {
    this.choose(this._matches[this.optionIndex(event)]);
  }

  /** The pointer chooses the option it is over, as the arrows would. */
  private optionHovered(event: Event): void {
    const index = this.optionIndex(event);
    if (index < 0 || index === this._chosen) {
      return;
    }
    this.markChosen(index);
  }

  /** A command typed whole, or chosen in the list, runs; the field empties, the pictures waiting stay. */
  private run(command: CommandLine): void {
    this._runs[command.name](command.argument);
  }

  private emitCommand(event: 'compact' | 'newSession'): void {
    this.clearText();
    this.events.emit(event);
  }

  private showHelp(): void {
    this.clearText();
    this._refs.help.show();
  }

  private clearText(): void {
    this._refs.text.value = '';
    this.typed();
  }

  /** The commands matching what is typed, the list hidden when none does. */
  private renderMenu(): void {
    const text = this._refs.text.value;
    this._matches = text === this._dismissed ? [] : menuMatching(text, this.commands.get(), this.skills.get());
    this._refs.commandList.hidden = this._matches.length === 0;
    this._refs.commandOptions.replaceChildren(
      ...this._matches.map((entry, index) => this.optionNode(entry, index))
    );
    this.markChosen(Math.min(this._chosen, Math.max(this._matches.length - 1, 0)));
  }

  /** Marks the option chosen in place, for the pointer and for screen readers, which the field tells which it is. */
  private markChosen(index: number): void {
    this._chosen = index;
    for (const option of this._refs.commandOptions.children) {
      option.setAttribute('aria-selected', String(option.getAttribute('data-index') === String(index)));
    }
    const field = this._refs.text;
    const chosen = this._matches[index];
    if (chosen === undefined) {
      field.removeAttribute('aria-controls');
      field.removeAttribute('aria-activedescendant');
      return;
    }
    field.setAttribute('aria-controls', COMMAND_LIST_ID);
    field.setAttribute('aria-activedescendant', optionId(chosen));
  }

  private optionNode(entry: MenuEntry, index: number): Node {
    const { argument, description, icon } =
      entry.kind === 'command'
        ? COMMANDS[entry.name]
        : { argument: undefined, description: entry.skill.description, icon: SKILL_ICON };
    const name = entry.kind === 'command' ? entry.name : entry.skill.name;
    return (
      // Focus stays in the field, which names the option chosen: an option is focusable by script only.
      <div role="option" id={optionId(entry)} class="option" data-index={String(index)} tabIndex={-1}>
        <JlIcon name={icon} size={COMMAND_ICON_SIZE} />
        <span class="name">
          {COMMAND_PREFIX}
          {name}
          {argument === undefined ? null : <span class="argument">{` ‹${argument}›`}</span>}
        </span>
        <span class="description">{description}</span>
      </div>
    );
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
    const command = commandLine(text, this.commands.get());
    if (command !== undefined) {
      this.run(command);
      return;
    }
    if (text === '' && this._pictures.length === 0) {
      return;
    }
    const draft = { pictures: this._pictures, text };
    this.clearText();
    this._pictures = [];
    this.renderPictures();
    this.events.emit('send', draft);
  }

  /** A question beside the session goes, the pictures waiting stay for the next message. */
  private peek(question: string): void {
    if (question === '') {
      return;
    }
    this.clearText();
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
