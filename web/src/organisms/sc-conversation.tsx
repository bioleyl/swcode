import { createElement, css } from '@jadis/core';
import { JlButton } from '@syware/jlib';

import { ENDPOINTS } from '../../../shared/protocol/endpoints';
import { ScElement } from '../base/sc-element';
import { activityLine } from '../lib/activity';
import { keepingEnd } from '../lib/follow-end';
import { folderName } from '../lib/format';
import { atTop, fillsWindow, scrollAfterOlder } from '../lib/history-window';
import { ScMessage } from '../molecules/sc-message';
import { ScPassword } from '../molecules/sc-password';
import { ScPermission } from '../molecules/sc-permission';
import { ScQuestion } from '../molecules/sc-question';
import { ScThinking } from '../molecules/sc-thinking';
import { ScToolRow } from '../molecules/sc-tool-row';
import { authorizedUrl } from '../services/access';
import { BASE_STYLES } from '../styles/shared';
import { ScCheckpointDiff } from './sc-checkpoint-diff';
import styles from './sc-conversation.css?inline';

import type { Streaming } from '../../../shared/protocol/messages.schema';
import type { PictureRef } from '../../../shared/protocol/picture.schema';
import type {
  DiffDisplay,
  PermissionDecision,
  ProjectNote,
  ReminderReason,
  SessionEvent,
  ToolName,
  TurnOutcome,
} from '../../../shared/protocol/session.schema';
import type { SkillSummary } from '../../../shared/protocol/skill.schema';
import type { MessageAuthor } from '../molecules/sc-message';
import type { Activity } from '../types/activity.types';
import type { SessionEventHandlers, SessionEventMap, ToolRowState } from '../types/conversation.types';

// How close to the end counts as "following along": new content then keeps the end in view.
const FOLLOW_DISTANCE_PX = 80;
// How close to the top counts as reaching it: the older events then come. A short way in, so a wheel turn that
// ends there asks once and the page it brings is already read as it lands above.
const TOP_DISTANCE_PX = 120;
// What the top of the conversation says while the older events come. Once they are all here the line goes away: the
// beginning of the conversation is then simply what is at the top.
const TOP_LOADING = 'Looking for earlier messages…';
// How long the message a pin points at stays marked once the view reached it.
const PIN_FLASH_MS = 1_200;
// A search's match is marked longer: the view may scroll a long way, through older pages, to reach it.
const MATCH_FLASH_MS = 2_400;

// The events kept for a row an older page has yet to bring, by what they wait for: a call's row, or a subagent's call.
const callKey = (callId: string): string => `call:${callId}`;
const agentKey = (agentId: string): string => `agent:${agentId}`;
// When the waiting messages go: after the turn, or at the agent's next step once the person asked for them now.
const QUEUE_NOTES = { later: 'Sent when this turn ends.', now: 'Sent at the agent’s next step.' } as const;

// What swcode told the model in the middle of a turn, and why.
const REMINDER_TITLES: Readonly<Record<ReminderReason, string>> = {
  plan: 'The agent stopped with steps of its plan left: swcode reminded it',
  stepLimit: 'The agent reached the step limit: swcode asked it for its report',
};

// The project's notes as the model read them, folded: their files are long, and told once a session.
const NOTES_LINE = 'The agent read the project’s notes (AGENTS.md)';
// Notes longer than a session reads lose their oldest: the person is the one who can have them tidied.
const NOTES_CUT_LINE = `${NOTES_LINE}, without their oldest: they are longer than a session reads`;

const notesLine = (notes: readonly ProjectNote[]): string =>
  notes.some((note) => note.leftOut !== undefined) ? NOTES_CUT_LINE : NOTES_LINE;

const skillsLine = (skills: readonly SkillSummary[]): string =>
  skills.length === 0
    ? 'The agent was told there are no skills now'
    : `Skills the agent was told of: ${skills.map((skill) => skill.name).join(', ')}`;

// What a subagent's row says beside its report, by how it ended.
const REPORT_METAS: Readonly<Record<TurnOutcome, string | undefined>> = {
  done: undefined,
  failed: 'failed',
  limit: 'too many steps',
  stopped: 'stopped',
};

const ENDINGS: Readonly<Record<TurnOutcome, string>> = {
  done: '',
  failed: 'The turn failed.',
  limit: 'Stopped: too many steps.',
  stopped: 'Stopped.',
};

const RESTORE_HINT = 'Restore the files this turn wrote or edited';
const CHANGES_HINT = 'See the files as they were before this turn';

const savedText = (count: number): string =>
  count === 1 ? '1 file saved before this turn' : `${count} files saved before this turn`;

const restoredText = (count: number): string =>
  count === 1 ? 'Restored 1 file to before that turn.' : `Restored ${count} files to before that turn.`;

/**
 * The conversation of a session, drawn from its events as they come: messages, tool calls, the permission
 * requests waiting for a person, and a line saying what the agent does while it works. It scrolls itself.
 */
export class ScConversation extends ScElement {
  static readonly selector = 'sc-conversation';

  readonly events = this.useEvents<{
    answer: { requestId: string; decision: PermissionDecision };
    cancelQueue: undefined;
    /** The person asked to see the files a turn changed: its id. */
    checkpointDiff: string;
    /** The person reached the top of what is shown: `older` events are missing above it. */
    older: number;
    password: { requestId: string; secret: string };
    question: { requestId: string; answers: string[] };
    /** The person asked to put back the files a turn changed: its id. */
    restoreCheckpoint: string;
    sendQueueNow: undefined;
    /** The person pinned a message, or took its pin off: which, and whether it is pinned now. */
    togglePin: { eventId: string; pinned: boolean };
  }>();

  /** Messages sent while a turn runs, waiting for the next one. */
  readonly queued = this.useChange<string[]>([], (messages) => this.renderQueued(messages));
  /** The person asked for them now: they go in at the agent's next step. */
  readonly queuedNow = this.useChange(false, (now) => this.renderQueueNote(now), { immediate: true });
  /** The messages the person pinned: each one's pin button says it. */
  readonly pinned = this.useChange<string[]>([], (ids) => this.renderPinned(ids), { immediate: true });

  /** Where the agent works, for the empty conversation's invitation. */
  readonly workspace = this.useChange('', (value) => {
    this._refs.where.textContent = value;
  });
  /** The session's id: the pictures pasted in it are its own. */
  readonly session = this.useChange('', () => undefined);

  // The events the session holds before the ones shown, told by the daemon: none means the conversation is whole.
  private _older = 0;
  // What the top says about the older events: waiting for them, or not waiting.
  private _topState: 'loading' | 'none' = 'none';
  // The rows an older page makes while its events are handled: they then go above what is shown.
  private _olderRows: Element[] | undefined;
  // The events that came before the row they change: the page shown starts after that row's call, which an older
  // page brings later. They are handled again then, in the order they came.
  private readonly _early = new Map<string, SessionEvent[]>();
  // The requests answered before the page that asked them came: they open no card.
  private readonly _answered = new Set<string>();
  // The ids of the pinned messages, and the message a pin asked to reach before its page came.
  private _pinned = new Set<string>();
  private _reveal = '';
  // How long the row reached stays marked: a search's match a while longer than a pin's.
  private _flash = PIN_FLASH_MS;

  private readonly _messages = new Map<string, ScMessage>();
  // The thoughts above them, by their event's id, which streams as one message's does.
  private readonly _thoughts = new Map<string, ScThinking>();
  private readonly _rows = new Map<string, ScToolRow>();
  // What waits for the person, by request: an approval, the agent's questions, or a password a command asked for.
  private readonly _requests = new Map<string, ScPermission | ScQuestion | ScPassword>();
  // Which agent call started each subagent: its calls show under that row.
  private readonly _subagents = new Map<string, string>();
  // The agent calls whose subagent works in the background: their rows run until its report, past the call's result.
  private readonly _working = new Set<string>();
  // What the agent does now, for the line under the conversation; set at every streamed token, so not a useChange.
  private _activity: Activity = { kind: 'working' };
  // What streamed in since the last frame, by message: an answer's words, and the thoughts above it. A model
  // writes many pieces a frame: they are drawn together, once, and not at all while the window is hidden.
  private readonly _streamed = new Map<string, string>();
  private readonly _reasoned = new Map<string, string>();
  private _frame = 0;

  private readonly _handlers: SessionEventHandlers = {
    assistantMessage: (event) => {
      if (event.reasoning) {
        this.thinking(event.id).finish(event.reasoning);
      }
      this.finishMessage(event.id, event.text);
      this.doing({ kind: 'working' });
    },
    checkpointRestored: (event) => this.note(restoredText(event.files.length), 'done'),
    checkpointSaved: (event) => this.add(this.checkpointNode(event.turnId, event.files)),
    contextCompacted: (event) => {
      const summary = (
        <details class="compaction">
          <summary>Earlier messages were summarized to save context</summary>
          <p>{event.summary}</p>
        </details>
      );
      summary.dataset.event = event.id;
      this.add(summary);
    },
    passwordAnswered: (event) => this.closeRequest(event.requestId),
    passwordRequested: (event) => this.openPassword(event),
    permissionAnswered: (event) => this.closeRequest(event.requestId),
    permissionRequested: (event) => this.openRequest(event),
    projectNotesRead: (event) =>
      this.add(
        <details class="compaction">
          <summary>{notesLine(event.notes)}</summary>
          {event.notes.map((note) => (
            <p>
              <strong>{note.path}</strong>
              {`\n${note.text}`}
            </p>
          ))}
        </details>
      ),
    questionAnswered: (event) => this.closeRequest(event.requestId),
    questionAsked: (event) => this.openQuestion(event),
    reminder: (event) =>
      this.add(
        <details class="compaction">
          <summary>{REMINDER_TITLES[event.reason]}</summary>
          <p>{event.text}</p>
        </details>
      ),
    skillLoaded: (event) =>
      this.add(
        <details class="compaction">
          <summary>{`Ran the skill ${event.name}`}</summary>
          <p>{event.text}</p>
        </details>
      ),
    skillsListed: (event) =>
      this.add(
        <details class="compaction">
          <summary>{skillsLine(event.skills)}</summary>
          <ul>
            {event.skills.map((skill) => (
              <li>
                <strong>{skill.name}</strong>: {skill.description}
              </li>
            ))}
          </ul>
        </details>
      ),
    subagentActivity: (event) => {
      const line = { meta: event.meta, ok: event.ok, summary: event.summary, tool: event.tool };
      this.subagentRow(event.agentId, event)?.activity.set((lines) => [...lines, line]);
    },
    subagentFinished: (event) => this.finishSubagent(event),
    // What the main agent read, where it read it: the report itself shows in its subagent's row.
    subagentQueued: (event) => {
      this._subagents.set(event.agentId, event.callId);
      this._working.add(event.callId);
      this.progress(this.rowOrLater(event.callId, event), 'queued');
      this.handleEarly(agentKey(event.agentId));
    },
    subagentReport: (event) =>
      this.add(
        <details class="compaction">
          <summary>{`Report of “${event.description}”, given to the agent`}</summary>
          <p>{event.text}</p>
          {event.note === undefined ? null : <p>{event.note}</p>}
        </details>
      ),
    subagentStarted: (event) => {
      this._subagents.set(event.agentId, event.callId);
      this._working.add(event.callId);
      this.progress(this.rowOrLater(event.callId, event), 'running');
      this.handleEarly(agentKey(event.agentId));
    },
    toolCall: (event) => {
      const row: ScToolRow = <ScToolRow tool={event.tool} summary={event.summary} />;
      row.dataset.event = event.callId;
      this._rows.set(event.callId, row);
      this.add(row);
      this.doing({ kind: 'running', summary: event.summary, tool: event.tool });
      this.handleEarly(callKey(event.callId));
    },
    toolFinished: (event) => {
      if (this._working.has(event.callId)) {
        this.doing({ kind: 'working' });
        return;
      }
      const row = this.rowOrLater(event.callId, event);
      row?.result.set({ display: event.display, meta: event.meta, ok: event.ok });
      row?.state.set(event.ok ? 'done' : 'failed');
      this.doing({ kind: 'working' });
    },
    toolRunning: (event) => this.runningRow(event),
    turnFinished: (event) => {
      this.working(false);
      this.note(event.message ?? ENDINGS[event.outcome], event.outcome);
    },
    turnStarted: () => {
      this.doing({ kind: 'working' });
      this.working(true);
    },
    userMessage: (event) => {
      const message = this.pinnableMessage(event.id, 'user');
      message.text.set(event.text);
      message.pictures.set((event.pictures ?? []).map((picture) => this.pictureUrl(picture)));
      this._messages.set(event.id, message);
      this.add(message);
    },
  };

  private readonly _refs = this.useRefs((ref) => ({
    cancel: ref<JlButton>('.cancel'),
    column: ref('.column'),
    diffs: ref(ScCheckpointDiff),
    empty: ref('.empty'),
    items: ref('.items'),
    queued: ref('.queued'),
    queuedMessages: ref('.queued-messages'),
    queuedWhen: ref('.queued-when'),
    sendNow: ref<JlButton>('.send-now'),
    status: ref('.working-text'),
    top: ref('.older'),
    where: ref('.where'),
    working: ref('.working'),
  }));

  templateHtml(): Node {
    return (
      <div class="column">
        <p class="empty">
          Ask swcode to look into something in <span class="where" />.
        </p>
        <div class="older" hidden />
        <div class="items" />
        <p class="working" hidden>
          <span class="dot" aria-hidden="true" />
          <span class="working-text" role="status" />
        </p>
        <div class="queued" hidden>
          <div class="queued-messages" />
          <p class="queued-note">
            <span class="queued-when" />
            <JlButton class="send-now" label="Send now" variant="ghost" size="sm" />
            <JlButton class="cancel" label="Cancel" variant="ghost" size="sm" />
          </p>
        </div>
        <ScCheckpointDiff />
      </div>
    );
  }

  templateCss(): string {
    return css`${BASE_STYLES}${styles}`;
  }

  onConnect(): void {
    this._refs.cancel.events.register('press', () => this.events.emit('cancelQueue'));
    this._refs.sendNow.events.register('press', () => this.events.emit('sendQueueNow'));
    this.on(this, 'scroll', () => this.watchTop());
  }

  onDisconnect(): void {
    cancelAnimationFrame(this._frame);
    this._frame = 0;
    this._streamed.clear();
    this._reasoned.clear();
  }

  /** Draws the end of a session again, from the history the daemon answered, and the answer being written. */
  load(events: readonly SessionEvent[], older: number, streaming: Streaming | undefined): void {
    this._older = older;
    this._reveal = '';
    this._early.clear();
    this._answered.clear();
    this._streamed.clear();
    this._reasoned.clear();
    this._messages.clear();
    this._thoughts.clear();
    this._rows.clear();
    this._requests.clear();
    this._subagents.clear();
    this._working.clear();
    this._refs.items.replaceChildren();
    this._refs.empty.hidden = events.length > 0;
    this.working(false);
    for (const event of events) {
      this.handle(event.kind, event);
    }
    if (streaming) {
      this.delta(streaming.messageId, streaming.text);
      this.drawStreamed();
    }
    this.scrollTop = this.scrollHeight;
    this._topState = 'none';
    this.showTop();
    this.askForRoom();
  }

  /** Adds a page of older events above what is shown, keeping people on what they were reading. */
  addOlder(events: readonly SessionEvent[], older: number): void {
    this._older = older;
    const atEnd = this.isAtEnd();
    const now = { activity: this._activity, working: !this._refs.working.hidden };
    this._refs.items.prepend(...this.rowsOf(events));
    // The page's turns are over: the line under the conversation goes on saying what the agent does now.
    this._activity = now.activity;
    this.working(now.working);
    this.scrollTop = scrollAfterOlder(this, atEnd);
    this._topState = 'none';
    this.showTop();
    this.askForRoom();
    this.revealShown();
  }

  /**
   * Goes on after a time offline: the events the session recorded meanwhile, and the answer being written as it
   * stands now. An older page asked for before the socket went never came: the top asks again when it is reached.
   */
  resume(events: readonly SessionEvent[], streaming: Streaming | undefined): void {
    this.drawStreamed();
    this.following(() => {
      for (const event of events) {
        this.handle(event.kind, event);
      }
      if (streaming) {
        this.message(streaming.messageId).text.set(streaming.text);
      }
    });
    this._topState = 'none';
    this.showTop();
  }

  /** An event of the session: what streamed before it is drawn first, a reply's whole text then replaces it. */
  apply(event: SessionEvent): void {
    this.drawStreamed();
    this.following(() => this.handle(event.kind, event));
  }

  delta(messageId: string, text: string): void {
    this._streamed.set(messageId, `${this._streamed.get(messageId) ?? ''}${text}`);
    this.drawNextFrame();
    this.doing({ kind: 'writing' });
  }

  reasoning(messageId: string, text: string): void {
    this._reasoned.set(messageId, `${this._reasoned.get(messageId) ?? ''}${text}`);
    this.drawNextFrame();
    this.doing({ kind: 'thinking' });
  }

  private drawNextFrame(): void {
    if (this._frame !== 0) {
      return;
    }
    this._frame = requestAnimationFrame(() => this.drawStreamed());
  }

  /** Draws what streamed in since the last frame: the thoughts first, which stand above their answer. */
  private drawStreamed(): void {
    cancelAnimationFrame(this._frame);
    this._frame = 0;
    if (this._streamed.size === 0 && this._reasoned.size === 0) {
      return;
    }
    this.following(() => {
      for (const [messageId, text] of this._reasoned) {
        this.thinking(messageId).append(text);
      }
      for (const [messageId, text] of this._streamed) {
        this.message(messageId).append(text);
      }
    });
    this._reasoned.clear();
    this._streamed.clear();
  }

  /** What a call's command printed since the last time, while it runs: its row shows the end of it. */
  showOutput(callId: string, text: string): void {
    this.following(() => this._rows.get(callId)?.printed(text));
  }

  /** The model started writing a call to this tool: its arguments come unseen, until the call is whole. */
  writingCall(tool: ToolName): void {
    this.doing({ kind: 'calling', tool });
  }

  private handle<Kind extends keyof SessionEventMap>(kind: Kind, event: SessionEventMap[Kind]): void {
    this._handlers[kind](event);
  }

  /**
   * The rows a page of older events makes, handled oldest first as the session recorded them: a call's row is there
   * before its result, a request before its answer. A row its own page took away again (an answered request, a
   * reply that only called tools) is left out.
   */
  private rowsOf(events: readonly SessionEvent[]): Element[] {
    const made: Element[] = [];
    this._olderRows = made;
    for (const event of events) {
      this.handle(event.kind, event);
    }
    this._olderRows = undefined;
    return made.filter((row) => row.parentNode === this._refs.items);
  }

  /** Keeps an event whose row is not drawn yet, for the older page that brings the row. */
  private keepEarly(key: string, event: SessionEvent): void {
    this._early.set(key, [...(this._early.get(key) ?? []), event]);
  }

  /** Handles the events that waited for what just came, in the order they came. */
  private handleEarly(key: string): void {
    const waiting = this._early.get(key) ?? [];
    this._early.delete(key);
    for (const event of waiting) {
      this.handle(event.kind, event);
    }
  }

  /**
   * Where a call stands while it has no result. A result is final: an older page can bring a call's start after the
   * result the page shown already gave its row, and the row stays finished.
   */
  private progress(row: ScToolRow | undefined, state: ToolRowState): void {
    if (row && row.result.get() === undefined) {
      row.state.set(state);
    }
  }

  /** The row of a call; when an older page has yet to bring it, none, and the event waits for it. */
  private rowOrLater(callId: string, event: SessionEvent): ScToolRow | undefined {
    const row = this._rows.get(callId);
    if (!row) {
      this.keepEarly(callKey(callId), event);
    }
    return row;
  }

  /** Reaches the top of what is shown: the older events come, once, until the session's whole list is here. */
  private watchTop(): void {
    if (this._older === 0 || this._topState !== 'none' || !atTop(this, TOP_DISTANCE_PX)) {
      return;
    }
    this.askOlder();
  }

  /** Asks for the page of older events above what is shown, once: its answer is added above it. */
  private askOlder(): void {
    this._topState = 'loading';
    this.showTop();
    this.events.emit('older', this._older);
  }

  /** Brings the message a pin points at into view, asking for the older pages before it while it is not drawn. */
  reveal(eventId: string): void {
    this.revealRow(eventId, PIN_FLASH_MS);
  }

  /** Brings the event a search found into view, marked longer: the view may scroll a long way to reach it. */
  revealMatch(eventId: string): void {
    this.revealRow(eventId, MATCH_FLASH_MS);
  }

  private revealRow(eventId: string, flashMs: number): void {
    this._reveal = eventId;
    this._flash = flashMs;
    this.revealShown();
  }

  private revealShown(): void {
    if (this._reveal === '') {
      return;
    }
    const row = this._refs.items.querySelector<HTMLElement>(`[data-event="${CSS.escape(this._reveal)}"]`);
    if (row) {
      this._reveal = '';
      this.shine(row);
      return;
    }
    if (this._older === 0) {
      this._reveal = '';
      return;
    }
    if (this._topState === 'none') {
      this.askOlder();
    }
  }

  /** Marks a message a moment, where the view landed. */
  private shine(row: HTMLElement): void {
    // A summary's words are folded: the row opens, so what a search found shows.
    if (row instanceof HTMLDetailsElement) {
      row.open = true;
    }
    row.scrollIntoView({ behavior: 'smooth', block: 'center' });
    row.classList.add('found');
    setTimeout(() => row.classList.remove('found'), this._flash);
  }

  /** The beginning of a conversation too short to fill the window comes by itself: no scrollbar to reach it with. */
  private askForRoom(): void {
    const drawn = this._refs.column.getBoundingClientRect().height;
    if (this._topState === 'none' && fillsWindow(this, this._older, drawn)) {
      this.watchTop();
    }
  }

  /** Whether people are looking at the end of the conversation, where a new event would show. */
  private isAtEnd(): boolean {
    return this.scrollHeight - this.scrollTop - this.clientHeight < FOLLOW_DISTANCE_PX;
  }

  /** What the top of the conversation says about the older events: that they are coming. */
  private showTop(): void {
    this._refs.top.hidden = this._topState === 'none';
    this._refs.top.textContent = this._topState === 'none' ? '' : TOP_LOADING;
  }

  /** Keeps the end in view when people were looking at it; leaves them where they are when they scrolled up. */
  private following(change: () => void): void {
    keepingEnd(this, FOLLOW_DISTANCE_PX, change);
  }

  private runningRow(event: SessionEventMap['toolRunning']): void {
    const row = this.rowOrLater(event.callId, event);
    if (!row) {
      return;
    }
    this.progress(row, 'running');
    this.doing({ kind: 'running', summary: row.summary.get(), tool: row.tool.get() });
  }

  /**
   * A subagent's end: its row shows its report, and how it ended. One from before subagents worked in the background
   * has none: the agent call's own result, which follows, holds its answer.
   */
  private finishSubagent(event: SessionEventMap['subagentFinished']): void {
    const callId = this._subagents.get(event.agentId);
    if (callId === undefined) {
      this.keepEarly(agentKey(event.agentId), event);
      return;
    }
    const row = this.rowOrLater(callId, event);
    this._working.delete(callId);
    if (!row || event.report === undefined) {
      return;
    }
    const done = event.outcome === 'done';
    row.result.set({
      display: { kind: 'text', text: event.report },
      meta: REPORT_METAS[event.outcome],
      ok: done,
    });
    row.state.set(done ? 'done' : 'failed');
  }

  /** The row of the call that started a subagent; none while an older page has yet to bring it: the event waits. */
  private subagentRow(agentId: string, event: SessionEvent): ScToolRow | undefined {
    const callId = this._subagents.get(agentId);
    if (callId === undefined) {
      this.keepEarly(agentKey(agentId), event);
      return undefined;
    }
    return this.rowOrLater(callId, event);
  }

  private pictureUrl(picture: PictureRef): string {
    return authorizedUrl(`${ENDPOINTS.images}/${encodeURIComponent(this.session.get())}/${picture.file}`);
  }

  private add(row: Element): void {
    this._olderRows?.push(row);
    this._refs.items.append(row);
    this._refs.empty.hidden = true;
  }

  private message(id: string): ScMessage {
    const known = this._messages.get(id);
    if (known) {
      return known;
    }
    const message = this.pinnableMessage(id, 'assistant');
    this._messages.set(id, message);
    this.add(message);
    return message;
  }

  /** A message of the conversation, which can be pinned: its event is where a pin points, and the button toggles it. */
  private pinnableMessage(id: string, author: MessageAuthor): ScMessage {
    const message: ScMessage = <ScMessage author={author} />;
    message.dataset.event = id;
    message.pinnable.set(true);
    message.pinned.set(this._pinned.has(id));
    message.events.register('pin', () =>
      this.events.emit('togglePin', { eventId: id, pinned: !message.pinned.get() })
    );
    return message;
  }

  private thinking(id: string): ScThinking {
    const known = this._thoughts.get(id);
    if (known) {
      return known;
    }
    const thinking: ScThinking = <ScThinking />;
    this._thoughts.set(id, thinking);
    this.add(thinking);
    return thinking;
  }

  /** The whole answer replaces what streamed; a reply that only calls tools leaves no empty message behind. */
  private finishMessage(id: string, text: string): void {
    if (text === '') {
      this._messages.get(id)?.remove();
      this._messages.delete(id);
      return;
    }
    this.message(id).text.set(text);
  }

  private openRequest(event: SessionEventMap['permissionRequested']): void {
    if (this._answered.delete(event.requestId)) {
      return;
    }
    const row = this._rows.get(event.callId);
    this.progress(row, 'waiting');
    const card: ScPermission = (
      <ScPermission
        tool={row?.tool.get()}
        summary={row?.summary.get() ?? ''}
        detail={event.detail}
        where={`main agent · in ${folderName(this.workspace.get())}`}
      />
    );
    this._requests.set(event.requestId, card);
    // Listens once the card is in the page, on the signal of its connection.
    this.add(card);
    card.events.register('answer', (decision) =>
      this.events.emit('answer', { decision, requestId: event.requestId })
    );
    this.working(true);
  }

  /** A command of this computer stopped asking for a password: the person types it, or says none is coming. */
  private openPassword(event: SessionEventMap['passwordRequested']): void {
    if (this._answered.delete(event.requestId)) {
      return;
    }
    this.progress(this._rows.get(event.callId), 'waiting');
    const card: ScPassword = (
      <ScPassword prompt={event.prompt} where={`main agent · in ${folderName(this.workspace.get())}`} />
    );
    this._requests.set(event.requestId, card);
    this.add(card);
    card.events.register('answer', (secret) =>
      this.events.emit('password', { requestId: event.requestId, secret })
    );
    card.events.register('refuse', () => this.events.emit('password', { requestId: event.requestId, secret: '' }));
    this.working(true);
  }

  private openQuestion(event: SessionEventMap['questionAsked']): void {
    if (this._answered.delete(event.requestId)) {
      return;
    }
    this.progress(this._rows.get(event.callId), 'waiting');
    const card: ScQuestion = (
      <ScQuestion questions={event.questions} where={`main agent · in ${folderName(this.workspace.get())}`} />
    );
    this._requests.set(event.requestId, card);
    this.add(card);
    card.events.register('answer', (answers) =>
      this.events.emit('question', { answers, requestId: event.requestId })
    );
    this.working(true);
  }

  /** A request answered: its card goes. One whose page has yet to come is remembered, and opens no card then. */
  private closeRequest(requestId: string): void {
    const card = this._requests.get(requestId);
    if (!card) {
      this._answered.add(requestId);
    }
    card?.remove();
    this._requests.delete(requestId);
    this.working(true);
  }

  private renderQueueNote(now: boolean): void {
    this._refs.queuedWhen.textContent = now ? QUEUE_NOTES.now : QUEUE_NOTES.later;
    this._refs.sendNow.hidden = now;
  }

  private renderPinned(ids: readonly string[]): void {
    this._pinned = new Set(ids);
    for (const [id, message] of this._messages) {
      message.pinned.set(this._pinned.has(id));
    }
  }

  private renderQueued(messages: readonly string[]): void {
    this._refs.queued.hidden = messages.length === 0;
    this._refs.queuedMessages.replaceChildren(...messages.map((text) => <ScMessage author="user" text={text} />));
  }

  private working(on: boolean): void {
    this._refs.working.hidden = !on;
    this.reflect('waiting', on && this._requests.size > 0);
    this.renderActivity();
  }

  private doing(activity: Activity): void {
    this._activity = activity;
    this.renderActivity();
  }

  /** Writes the line only when its words change: a screen reader reads the status out at each change. */
  private renderActivity(): void {
    const line = this._requests.size > 0 ? 'Waiting for you' : activityLine(this._activity);
    if (this._refs.status.textContent === line) {
      return;
    }
    this._refs.status.textContent = line;
  }

  /** The files a turn changed, with the buttons that see them as they were, and put them back. */
  private checkpointNode(turnId: string, files: readonly string[]): Element {
    const changes = createElement(JlButton, {
      attrs: { class: 'changes' },
      props: { hint: CHANGES_HINT, label: 'Changes', size: 'sm', variant: 'ghost' },
    });
    changes.events.register('press', () => this.events.emit('checkpointDiff', turnId));
    const restore = createElement(JlButton, {
      attrs: { class: 'restore' },
      props: { hint: RESTORE_HINT, label: 'Restore', size: 'sm', variant: 'ghost' },
    });
    restore.events.register('press', () => this.events.emit('restoreCheckpoint', turnId));
    const row = (
      <p class="checkpoint">
        <span class="checkpoint-text">{savedText(files.length)}</span>
      </p>
    );
    row.append(changes, restore);
    return row;
  }

  /** The files of a turn's restore point, as diffs, in their dialog. */
  showCheckpointDiff(diffs: readonly DiffDisplay[]): void {
    this._refs.diffs.show(diffs);
  }

  private note(text: string, outcome: TurnOutcome): void {
    if (text === '') {
      return;
    }
    this.add(
      <p class="note" data-outcome={outcome}>
        {text}
      </p>
    );
  }
}

ScConversation.register();
