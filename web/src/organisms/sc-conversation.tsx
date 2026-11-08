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
import styles from './sc-conversation.css?inline';

import type { Streaming } from '../../../shared/protocol/messages.schema';
import type { PictureRef } from '../../../shared/protocol/picture.schema';
import type {
  PermissionDecision,
  ReminderReason,
  SessionEvent,
  ToolName,
  TurnOutcome,
} from '../../../shared/protocol/session.schema';
import type { SkillSummary } from '../../../shared/protocol/skill.schema';
import type { Activity } from '../types/activity.types';
import type { SessionEventHandlers, SessionEventMap } from '../types/conversation.types';

// How close to the end counts as "following along": new content then keeps the end in view.
const FOLLOW_DISTANCE_PX = 80;
// How close to the top counts as reaching it: the older events then come. A short way in, so a wheel turn that
// ends there asks once and the page it brings is already read as it lands above.
const TOP_DISTANCE_PX = 120;
// What the top of the conversation says while the older events come. Once they are all here the line goes away: the
// beginning of the conversation is then simply what is at the top.
const TOP_LOADING = 'Looking for earlier messages…';

/** Holds rows together, for one of the rare events that draws several (a subagent's report has its note). */
const fragmentOf = (nodes: readonly Element[]): DocumentFragment => {
  const fragment = document.createDocumentFragment();
  fragment.append(...nodes);
  return fragment;
};
// When the waiting messages go: after the turn, or at the agent's next step once the person asked for them now.
const QUEUE_NOTES = { later: 'Sent when this turn ends.', now: 'Sent at the agent’s next step.' } as const;

// What swcode told the model in the middle of a turn, and why.
const REMINDER_TITLES: Readonly<Record<ReminderReason, string>> = {
  plan: 'The agent stopped with steps of its plan left: swcode reminded it',
  stepLimit: 'The agent reached the step limit: swcode asked it for its report',
};

// The project's notes as the model read them, folded: their files are long, and told once a session.
const NOTES_LINE = 'The agent read the project’s notes (AGENTS.md)';

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
    /** The person reached the top of what is shown: `older` events are missing above it. */
    older: number;
    password: { requestId: string; secret: string };
    question: { requestId: string; answers: string[] };
    /** The person asked to put back the files a turn changed: its id. */
    restoreCheckpoint: string;
    sendQueueNow: undefined;
  }>();

  /** Messages sent while a turn runs, waiting for the next one. */
  readonly queued = this.useChange<string[]>([], (messages) => this.renderQueued(messages));
  /** The person asked for them now: they go in at the agent's next step. */
  readonly queuedNow = this.useChange(false, (now) => this.renderQueueNote(now), { immediate: true });

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
  // The rows added since the last turn of the handlers: an older page takes them from the end and puts them above.
  private _pending: Element[] = [];

  private readonly _messages = new Map<string, ScMessage>();
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
    contextCompacted: (event) =>
      this.add(
        <details class="compaction">
          <summary>Earlier messages were summarized to save context</summary>
          <p>{event.summary}</p>
        </details>
      ),
    passwordAnswered: (event) => this.closeRequest(event.requestId),
    passwordRequested: (event) => this.openPassword(event),
    permissionAnswered: (event) => this.closeRequest(event.requestId),
    permissionRequested: (event) => this.openRequest(event),
    projectNotesRead: (event) =>
      this.add(
        <details class="compaction">
          <summary>{NOTES_LINE}</summary>
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
      this.subagentRow(event.agentId)?.activity.set((lines) => [...lines, line]);
    },
    subagentFinished: (event) => this.finishSubagent(event),
    // What the main agent read, where it read it: the report itself shows in its subagent's row.
    subagentQueued: (event) => {
      this._subagents.set(event.agentId, event.callId);
      this._working.add(event.callId);
      this._rows.get(event.callId)?.state.set('queued');
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
      this._rows.get(event.callId)?.state.set('running');
    },
    toolCall: (event) => {
      const row: ScToolRow = <ScToolRow tool={event.tool} summary={event.summary} />;
      this._rows.set(event.callId, row);
      this.add(row);
      this.doing({ kind: 'running', summary: event.summary, tool: event.tool });
    },
    toolFinished: (event) => {
      if (this._working.has(event.callId)) {
        this.doing({ kind: 'working' });
        return;
      }
      const row = this._rows.get(event.callId);
      row?.result.set({ display: event.display, meta: event.meta, ok: event.ok });
      row?.state.set(event.ok ? 'done' : 'failed');
      this.doing({ kind: 'working' });
    },
    toolRunning: (event) => this.runningRow(event.callId),
    turnFinished: (event) => {
      this.working(false);
      this.note(event.message ?? ENDINGS[event.outcome], event.outcome);
    },
    turnStarted: () => {
      this.doing({ kind: 'working' });
      this.working(true);
    },
    userMessage: (event) =>
      this.add(
        <ScMessage
          author="user"
          text={event.text}
          pictures={(event.pictures ?? []).map((picture) => this.pictureUrl(picture))}
        />
      ),
  };

  private readonly _refs = this.useRefs((ref) => ({
    cancel: ref<JlButton>('.cancel'),
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

  /** Draws the end of a session again, from the history the daemon answered, and the answer being written. */
  load(events: readonly SessionEvent[], older: number, streaming: Streaming | undefined): void {
    this._older = older;
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
    for (const event of [...events].reverse()) {
      const row = this.nodeOf(event);
      if (row) {
        this._refs.items.prepend(row);
      }
    }
    this.scrollTop = scrollAfterOlder(this, atEnd);
    this._topState = 'none';
    this.showTop();
    this.askForRoom();
  }

  apply(event: SessionEvent): void {
    this.following(() => this.handle(event.kind, event));
  }

  delta(messageId: string, text: string): void {
    this.following(() => this.message(messageId).append(text));
    this.doing({ kind: 'writing' });
  }

  reasoning(messageId: string, text: string): void {
    this.following(() => this.thinking(messageId).append(text));
    this.doing({ kind: 'thinking' });
  }

  /** The model started writing a call to this tool: its arguments come unseen, until the call is whole. */
  writingCall(tool: ToolName): void {
    this.doing({ kind: 'calling', tool });
  }

  private handle<Kind extends keyof SessionEventMap>(kind: Kind, event: SessionEventMap[Kind]): void {
    this._handlers[kind](event);
  }

  /** The row an event makes, without adding it: an older page goes above what is shown, not after it. Some events
   * answer a request that was already answered in the meantime and draw nothing at all. */
  private nodeOf(event: SessionEvent): Node | undefined {
    this._pending = [];
    this.handle(event.kind, event);
    const made = this._pending;
    this._pending = [];
    for (const node of made) {
      node.remove();
    }
    return made.length === 1 ? made[0] : made.length === 0 ? undefined : fragmentOf(made);
  }

  /** Reaches the top of what is shown: the older events come, once, until the session's whole list is here. */
  private watchTop(): void {
    if (this._older === 0 || this._topState !== 'none' || !atTop(this, TOP_DISTANCE_PX)) {
      return;
    }
    this._topState = 'loading';
    this.showTop();
    this.events.emit('older', this._older);
  }

  /** The beginning of a conversation too short to fill the window comes by itself: no scrollbar to reach it with. */
  private askForRoom(): void {
    if (this._topState === 'none' && fillsWindow(this, this._older)) {
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

  private runningRow(callId: string): void {
    const row = this._rows.get(callId);
    if (!row) {
      return;
    }
    row.state.set('running');
    this.doing({ kind: 'running', summary: row.summary.get(), tool: row.tool.get() });
  }

  /**
   * A subagent's end: its row shows its report, and how it ended. One from before subagents worked in the background
   * has none: the agent call's own result, which follows, holds its answer.
   */
  private finishSubagent(event: SessionEventMap['subagentFinished']): void {
    const callId = this._subagents.get(event.agentId);
    if (callId === undefined) {
      return;
    }
    this._working.delete(callId);
    if (event.report === undefined) {
      return;
    }
    const row = this._rows.get(callId);
    const done = event.outcome === 'done';
    row?.result.set({
      display: { kind: 'text', text: event.report },
      meta: REPORT_METAS[event.outcome],
      ok: done,
    });
    row?.state.set(done ? 'done' : 'failed');
  }

  private subagentRow(agentId: string): ScToolRow | undefined {
    const callId = this._subagents.get(agentId);
    return callId === undefined ? undefined : this._rows.get(callId);
  }

  private pictureUrl(picture: PictureRef): string {
    return authorizedUrl(`${ENDPOINTS.images}/${encodeURIComponent(this.session.get())}/${picture.file}`);
  }

  private add(row: Element): void {
    this._pending.push(row);
    this._refs.items.append(row);
    this._refs.empty.hidden = true;
  }

  private message(id: string): ScMessage {
    const known = this._messages.get(id);
    if (known) {
      return known;
    }
    const message: ScMessage = <ScMessage author="assistant" />;
    this._messages.set(id, message);
    this.add(message);
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
    const row = this._rows.get(event.callId);
    row?.state.set('waiting');
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
    this._rows.get(event.callId)?.state.set('waiting');
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
    this._rows.get(event.callId)?.state.set('waiting');
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

  private closeRequest(requestId: string): void {
    this._requests.get(requestId)?.remove();
    this._requests.delete(requestId);
    this.working(true);
  }

  private renderQueueNote(now: boolean): void {
    this._refs.queuedWhen.textContent = now ? QUEUE_NOTES.now : QUEUE_NOTES.later;
    this._refs.sendNow.hidden = now;
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

  /** The files a turn changed, with the button that puts them back as they were before it. */
  private checkpointNode(turnId: string, files: readonly string[]): Element {
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
    row.append(restore);
    return row;
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
