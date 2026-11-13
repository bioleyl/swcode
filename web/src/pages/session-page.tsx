import { css } from '@jadis/core';
import { JlAlert, JlBadge, JlButton, JlIcon, toast } from '@syware/jlib';

import { MAX_TERMINAL_TITLE } from '../../../shared/protocol/messages.schema';
import { ScElement } from '../base/sc-element';
import { agentsOf } from '../lib/agents';
import { SESSION_COMMANDS } from '../lib/commands';
import { contextText, speedLine } from '../lib/format';
import { currentPlan } from '../lib/plan';
import { presetOf, presetStatusOf } from '../lib/presets';
import { skillsOn } from '../lib/project-skills';
import { sessionPlace } from '../lib/session-kinds';
import { canDeleteSession, deleteHintOf, liveStatus, SESSION_STATUS_LOOKS } from '../lib/session-status';
import { ScSessionTodos } from '../molecules/sc-session-todos';
import { ScTitle } from '../molecules/sc-title';
import { ScArcade } from '../organisms/sc-arcade';
import { ScComposer } from '../organisms/sc-composer';
import { ScConversation } from '../organisms/sc-conversation';
import { ScPeek } from '../organisms/sc-peek';
import { ScPlan } from '../organisms/sc-plan';
import { ScReview } from '../organisms/sc-review';
import { ScSidePanel } from '../organisms/sc-side-panel';
import { ScTerminal } from '../organisms/sc-terminal';
import { daemon, daemonBus } from '../services/daemon';
import { daemonState, daemonStatusBus } from '../services/daemon-status';
import { navigate, newPathOf } from '../services/navigation';
import { presetChoices, presetsBus } from '../services/presets';
import { projectAt, projectListBus } from '../services/project-list';
import { foundSkills, projectSkillsBus, readProjectSkills } from '../services/project-skills';
import { addProjectTodo } from '../services/project-todos';
import { markSeen } from '../services/seen-sessions';
import { deleteSession } from '../services/session-delete';
import { findSession, sessionListBus } from '../services/session-list';
import { restoreCheckpoint } from '../services/session-restore';
import { sessionRevealBus, takeReveal } from '../services/session-reveal';
import { sessionLeft, sessionShown } from '../services/session-stream';
import { setSidePanelOpen, sidePanelOpen } from '../services/side-panel';
import { allTerminals, terminalKey, terminalListBus, terminalsIn } from '../services/terminals';
import { BASE_STYLES } from '../styles/shared';
import styles from './session-page.css?inline';

import type {
  ClientMessage,
  HistoryPart,
  ServerMessage,
  Terminal,
} from '../../../shared/protocol/messages.schema';
import type {
  DiffDisplay,
  SessionEvent,
  SessionEventKind,
  SessionSummary,
} from '../../../shared/protocol/session.schema';
import type { PromptDraft } from '../types/composer.types';
import type { ServerHandlers, ServerMessageMap } from '../types/page.types';
import type { TerminalSize } from '../types/terminal.types';

const NOT_FOUND = 'This session is not on the daemon.';
const NO_PROJECT = 'This session works in no project: a todo needs one.';
const REVIEW_HINT = 'What the workspace has uncommitted, to review and commit';
const COMMITTED = 'The changes are committed';
const VISIBILITY_CHANGE = 'visibilitychange';
// On the host while the side panel is hidden.
const PANEL_CLOSED = 'panel-closed';
// What a terminal the person opens starts with, until its view says how big it is.
const NEW_TERMINAL_COLUMNS = 80;
const NEW_TERMINAL_ROWS = 24;
// How many events a session page opens with, and how many each scroll to the top brings: the same for both, so a
// page is enough to read through before the next is asked, and few enough for a long session to show at once.
const HISTORY_EVENTS = 120;
// The events that can change the plan above the composer: a new turn hides the last one, a plan call shows its own.
const PLAN_EVENTS: ReadonlySet<SessionEventKind> = new Set(['turnStarted', 'toolFinished']);
const SUBAGENT_EVENTS: ReadonlySet<SessionEventKind> = new Set([
  'subagentQueued',
  'subagentStarted',
  'subagentActivity',
  'subagentFinished',
]);

/**
 * The prompt tokens of the last reply, plus its own: how much of the context the conversation fills. None after a
 * summary, until the next reply counts again.
 */
const usedTokens = (events: readonly SessionEvent[]): number => {
  const last = events.findLast(
    (event) =>
      event.kind === 'contextCompacted' || (event.kind === 'assistantMessage' && event.usage !== undefined)
  );
  return last?.kind === 'assistantMessage' && last.usage
    ? last.usage.promptTokens + last.usage.completionTokens
    : 0;
};

/** The last reply's tokens and times, as llm-server gave them: how fast the model is now, and whether its cache holds. */
const lastUsage = (events: readonly SessionEvent[]) =>
  events.findLast((event) => event.kind === 'assistantMessage' && event.usage !== undefined);

/** The settings a session made by /new takes from the one it was typed in: a chat's place and permissions are none. */
const sameSettings = (session: SessionSummary): ClientMessage => ({
  concise: session.concise,
  kind: session.kind,
  thinking: session.thinking,
  type: 'createSession',
  ...(session.preset === '' ? {} : { preset: session.preset }),
  ...(session.kind === 'code' ? { host: session.host, mode: session.mode, workspace: session.workspace } : {}),
});

/**
 * One session: its conversation, the composer under it, and on a wide screen what the agent runs on. A chat shows no
 * folder, and has no /peek: its turns are short.
 */
export class SessionPage extends ScElement {
  static readonly selector = 'session-page';

  private _sessionId = '';
  // The peeks this page asked, to name each: their answers come to this page alone.
  private _peeks = 0;
  private _used = 0;
  // The project whose skills this page read for the composer: they are read again for another one.
  private _skillsOf: string | undefined;
  // The session's events the page holds, oldest first: the side panel lists its subagents from them, and its token
  // count and plan come from them. Older pages are added in front as the person scrolls up.
  private _events: SessionEvent[] = [];
  // How many events the session holds before the ones this page has: with them, how far into the session it is.
  private _keptBack = 0;
  // Whether this page has its history yet: a search's match is marked once it has.
  private _opened = false;
  // Whether this page counts among those that show its session, from its connection to its leaving.
  private _showing = false;
  // The event a search found in this session, until the conversation is drawn and marks it.
  private _found = '';
  // The terminal drawn in the conversation's room, if one is; and whether the next terminalOpened is one this page
  // just asked for, which it then shows.
  private _shownTerminal = '';
  private _openingTerminal = false;
  // What the panel lists and the view shows of the terminals: drawn again only when it changes, since the session's
  // list changes at every event of a turn.
  private _terminalsKey = '';

  readonly route = this.useAttributes({
    id: (id) => {
      this._sessionId = id ?? '';
      this._refs.conversation.session.set(this._sessionId);
      this._refs.composer.draftPlace.set(this._sessionId);
    },
  });

  private readonly _handlers: ServerHandlers = {
    assistantDelta: (message) =>
      this.forThisSession(message.sessionId, () => this._refs.conversation.delta(message.messageId, message.text)),
    // How a commit of the workspace went, for the review that asked for it.
    changesCommitted: (message) =>
      this.forThisSession(message.sessionId, () => this.committed(message.problem ?? '')),
    // A turn's restore point, as diffs, for the dialog the conversation shows.
    checkpointDiff: (message) =>
      this.forThisSession(message.sessionId, () =>
        this._refs.conversation.showCheckpointDiff(
          message.files.filter((file): file is DiffDisplay => file.kind === 'diff')
        )
      ),
    peekDelta: (message) =>
      this.forThisSession(message.sessionId, () => this._refs.peek.delta(message.peekId, message.text)),
    peekState: (message) =>
      this.forThisSession(message.sessionId, () =>
        this._refs.peek.showState(message.peekId, message.state, message.message ?? '')
      ),
    problem: (message) => this.showProblem(message.message),
    reasoningDelta: (message) =>
      this.forThisSession(message.sessionId, () =>
        this._refs.conversation.reasoning(message.messageId, message.text)
      ),
    // What the workspace has uncommitted, for the review the person opened.
    reviewChanges: (message) => this.forThisSession(message.sessionId, () => this._refs.review.show(message)),
    sessionEvent: (message) => this.forThisSession(message.sessionId, () => this.applyEvent(message.event)),
    sessionHistory: (message) =>
      this.forThisSession(message.sessionId, () => {
        this._histories[message.part](message);
        this._refs.panel.agents.set(agentsOf(this._events));
        this._refs.plan.show(currentPlan(this._events));
        this.showUsage(usedTokens(this._events));
        this.showSpeed(lastUsage(this._events));
      }),
    terminalOpened: (message) => this.terminalOpened(message.terminal, message.output),
    terminalOutput: (message) => {
      if (message.terminalId === this._shownTerminal) {
        this._refs.terminal.write(message.data);
      }
    },
    terminals: () => this.renderTerminals(),
    toolCallWriting: (message) =>
      this.forThisSession(message.sessionId, () => this._refs.conversation.writingCall(message.tool)),
    toolOutput: (message) =>
      this.forThisSession(message.sessionId, () =>
        this._refs.conversation.showOutput(message.callId, message.text)
      ),
  };

  // A part of the session's history, by which part it is: the end draws the conversation, an older page goes above
  // what is shown, and what came while the page was offline goes after it.
  private readonly _histories: Readonly<
    Record<HistoryPart, (history: ServerMessageMap['sessionHistory']) => void>
  > = {
    end: (history) => {
      this._opened = true;
      this._events = [...history.events];
      this._keptBack = history.keptBack;
      this._refs.conversation.load(history.events, history.keptBack, history.streaming);
      this.revealFound();
    },
    older: (history) => {
      this._events = [...history.events, ...this._events];
      this._keptBack = history.keptBack;
      this._refs.conversation.addOlder(history.events, history.keptBack);
    },
    since: (history) => {
      this._opened = true;
      this._events = [...this._events, ...history.events];
      this._refs.conversation.resume(history.events, history.streaming);
      this.revealFound();
    },
  };

  private readonly _refs = this.useRefs((ref) => ({
    arcade: ref(ScArcade),
    composer: ref(ScComposer),
    conversation: ref(ScConversation),
    panel: ref(ScSidePanel),
    panelToggle: ref<JlButton>('.panel-toggle'),
    path: ref('.path'),
    peek: ref(ScPeek),
    place: ref('.chip'),
    plan: ref(ScPlan),
    problem: ref(JlAlert),
    remove: ref<JlButton>('.delete'),
    review: ref(ScReview),
    reviewOpen: ref<JlButton>('.review'),
    status: ref(JlBadge),
    terminal: ref(ScTerminal),
    title: ref(ScTitle),
    todos: ref(ScSessionTodos),
  }));

  templateHtml(): Node {
    return (
      <>
        <div class="column">
          <header>
            <ScTitle />
            <JlBadge class="status" />
            <span class="chip">
              <JlIcon name="laptop" size={14} />
              <span class="path" />
            </span>
            <JlButton class="review" icon="file-diff" variant="ghost" size="sm" hint={REVIEW_HINT} hidden />
            <JlButton class="delete" icon="trash" variant="ghost" size="sm" hidden />
            <JlButton class="panel-toggle" variant="ghost" size="sm" />
          </header>
          <ScConversation />
          <ScTerminal />
          <JlAlert tone="danger" hidden />
          <ScPlan hidden />
          <ScPeek hidden />
          <ScSessionTodos hidden />
          <ScComposer />
        </div>
        <ScSidePanel />
        <ScArcade />
        <ScReview />
      </>
    );
  }

  templateCss(): string {
    return css`${BASE_STYLES}${styles}`;
  }

  onConnect(): void {
    this._showing = true;
    sessionShown(this._sessionId);
    this._opened = false;
    this._found = takeReveal(this._sessionId) ?? '';
    this._events = [];
    this._keptBack = 0;
    this.watchDaemon();
    this.wireConversation();
    this.wireComposer();
    this.wireFrame();
    this.wirePins();
    this.wireTerminals();
    this.renderSummary();
    this.renderDaemon();
    // Offline, the socket's opening asks: asking here too would bring the history twice.
    if (daemon.online) {
      this.openHistory();
    }
    this._refs.composer.focusText();
  }

  /** Leaving the page: the session's stream stops coming to this socket, and so does the terminal it showed. */
  onDisconnect(): void {
    // A page removed before it connected for good never said it showed the session.
    if (this._showing) {
      this._showing = false;
      sessionLeft(this._sessionId);
    }
    if (this._shownTerminal !== '') {
      daemon.send({ terminalId: this._shownTerminal, type: 'detachTerminal' });
    }
  }

  /** What the daemon says, and what it says about itself: the page redraws what it shows. */
  private watchDaemon(): void {
    this.onBus(daemonBus, 'message', (message) => this.receive(message));
    this.onBus(daemonBus, 'online', (online) => this.reopen(online));
    this.onBus(sessionListBus, 'changed', () => this.renderSummary());
    this.onBus(projectListBus, 'changed', () => {
      this.renderSkills();
      this.renderLinkedTodos();
    });
    this.onBus(projectSkillsBus, 'changed', () => this.renderSkills());
    this.on(document, VISIBILITY_CHANGE, () => this.noteSeen());
    this.onBus(daemonStatusBus, 'changed', () => this.renderDaemon());
    this.onBus(presetsBus, 'changed', () => this.renderDaemon());
  }

  /** The conversation's answers: each goes to the daemon as the answer to its own request. */
  private wireConversation(): void {
    const { conversation } = this._refs;
    conversation.events.register('answer', ({ decision, requestId }) =>
      daemon.send({ decision, requestId, sessionId: this._sessionId, type: 'answerPermission' })
    );
    conversation.events.register('question', ({ answers, requestId }) =>
      daemon.send({ answers, requestId, sessionId: this._sessionId, type: 'answerQuestion' })
    );
    conversation.events.register('password', ({ requestId, secret }) =>
      daemon.send({ requestId, secret, sessionId: this._sessionId, type: 'answerPassword' })
    );
    conversation.events.register('cancelQueue', () =>
      daemon.send({ sessionId: this._sessionId, type: 'clearQueue' })
    );
    conversation.events.register('sendQueueNow', () =>
      daemon.send({ sessionId: this._sessionId, type: 'sendQueueNow' })
    );
    conversation.events.register('restoreCheckpoint', (turnId) => void restoreCheckpoint(this._sessionId, turnId));
    conversation.events.register('checkpointDiff', (turnId) =>
      daemon.send({ sessionId: this._sessionId, turnId, type: 'checkpointDiff' })
    );
    conversation.events.register('openTerminal', (terminalId) => this.showTerminal(terminalId));
    conversation.events.register('older', (older) =>
      daemon.send({ events: HISTORY_EVENTS, keptBack: older, sessionId: this._sessionId, type: 'olderEvents' })
    );
    this.onBus(sessionRevealBus, 'asked', ({ eventId, sessionId }) =>
      this.forThisSession(sessionId, () => this.showFound(eventId))
    );
  }

  /** A search found an event in this session: it is marked now, or once the conversation is drawn. */
  private showFound(eventId: string): void {
    this._found = eventId;
    // This page has it now: the next page of this session has not to mark it again.
    takeReveal(this._sessionId);
    this.revealFound();
  }

  /** Brings the event a search found into view, once its row is drawn: the conversation marks it a moment. */
  private revealFound(): void {
    if (!this._opened || this._found === '') {
      return;
    }
    const eventId = this._found;
    this._found = '';
    this._refs.conversation.revealMatch(eventId);
  }

  /** The composer: its message, its commands, and the session settings its chip changes. */
  private wireComposer(): void {
    const { composer } = this._refs;
    composer.events.register('send', (draft) => this.send(draft));
    composer.events.register('peek', (question) => this.peek(question));
    composer.events.register('todo', (text) => this.addTodo(text));
    composer.events.register('newSession', () => this.startNew());
    composer.events.register('compact', () => {
      this.showProblem('');
      daemon.send({ sessionId: this._sessionId, type: 'compactSession' });
    });
    composer.events.register('stop', () => daemon.send({ sessionId: this._sessionId, type: 'stopTurn' }));
    composer.events.register('play', () => this._refs.arcade.open());
    composer.events.register('mode', (mode) => daemon.send({ mode, sessionId: this._sessionId, type: 'setMode' }));
    composer.events.register('preset', (preset) =>
      daemon.send({ preset, sessionId: this._sessionId, type: 'setPreset' })
    );
    composer.events.register('concise', (concise) =>
      daemon.send({ concise, sessionId: this._sessionId, type: 'setConcise' })
    );
    composer.events.register('thinking', (thinking) =>
      daemon.send({ sessionId: this._sessionId, thinking, type: 'setThinking' })
    );
  }

  /** The page's own parts: its title, its delete button, the side panel and its stop buttons. */
  private wireFrame(): void {
    this._refs.title.events.register('rename', (title) =>
      daemon.send({ sessionId: this._sessionId, title, type: 'renameSession' })
    );
    this._refs.remove.events.register('press', () => void this.deleteSession());
    this.wireReview();
    this._refs.panelToggle.events.register('press', () => this.showPanel(!sidePanelOpen()));
    this._refs.panel.events.register('stopJob', (jobId) =>
      daemon.send({ jobId, sessionId: this._sessionId, type: 'stopJob' })
    );
    this.renderPanel(sidePanelOpen());
  }

  /**
   * The review of the workspace: its button opens it, it asks for the changes and gives the commit's message, and
   * the daemon's answers come back to it through the handlers.
   */
  private wireReview(): void {
    const { review } = this._refs;
    this._refs.reviewOpen.events.register('press', () => review.open());
    review.events.register('ask', () => daemon.send({ sessionId: this._sessionId, type: 'reviewChanges' }));
    review.events.register('commit', (message) =>
      daemon.send({ message, sessionId: this._sessionId, type: 'commitChanges' })
    );
  }

  /** A commit of the workspace was made, or git refused it: the review says which, and a toast that it is done. */
  private committed(problem: string): void {
    this._refs.review.committed(problem);
    if (problem === '') {
      toast({ heading: COMMITTED, tone: 'ok' });
    }
  }

  /**
   * The pins: a message's own button adds or takes off its pin, the side panel opens a pinned message or takes its
   * pin off, and the daemon keeps them with the session.
   */
  private wirePins(): void {
    const { conversation, panel } = this._refs;
    conversation.events.register('togglePin', ({ eventId, pinned }) =>
      daemon.send(
        pinned
          ? { eventId, sessionId: this._sessionId, type: 'pinEvent' }
          : { eventId, sessionId: this._sessionId, type: 'unpinEvent' }
      )
    );
    panel.events.register('showPin', (eventId) => conversation.reveal(eventId));
    panel.events.register('removePin', (eventId) =>
      daemon.send({ eventId, sessionId: this._sessionId, type: 'unpinEvent' })
    );
  }

  /** The terminals: the panel opens, ends and starts them; the view sends what the person types and how big it is. */
  private wireTerminals(): void {
    const { panel, terminal } = this._refs;
    panel.events.register('newTerminal', () => this.openTerminal());
    panel.events.register('openTerminal', (terminalId) => this.showTerminal(terminalId));
    panel.events.register('closeTerminal', (terminalId) => daemon.send({ terminalId, type: 'closeTerminal' }));
    panel.events.register('renameTerminal', ({ terminalId, title }) =>
      daemon.send({ terminalId, title: title.slice(0, MAX_TERMINAL_TITLE), type: 'renameTerminal' })
    );
    terminal.events.register('input', (data) => this.typeInTerminal(data));
    terminal.events.register('resize', (size) => this.resizeTerminal(size));
    terminal.events.register('back', () => this.hideTerminal());
    terminal.events.register('close', () => this.closeShownTerminal());
    this.onBus(terminalListBus, 'changed', () => this.renderTerminals());
    this.renderTerminals();
  }

  /**
   * A terminal opened: the one this page asked for shows, sized to its view; another is one it asked to show again,
   * and its kept output redraws the view.
   */
  private terminalOpened(terminal: Terminal, output: string): void {
    if (this._openingTerminal) {
      this._openingTerminal = false;
      this.drawTerminal(terminal);
      this.resizeTerminal(this._refs.terminal.size());
      return;
    }
    if (terminal.id === this._shownTerminal) {
      this._refs.terminal.replay(output);
    }
  }

  /** The terminals of this session's project, as the daemon lists them; a chat has none. */
  private renderTerminals(): void {
    const session = findSession(this._sessionId);
    const code = session?.kind === 'code';
    const found = session && code ? terminalsIn(session.workspace, session.host) : [];
    const key = `${code} ${found.map(terminalKey).join(',')}`;
    if (key === this._terminalsKey) {
      return;
    }
    this._terminalsKey = key;
    this._refs.panel.canTerminal.set(code);
    this._refs.panel.terminals.set(found);
    if (this._shownTerminal === '') {
      return;
    }
    const shown = found.find((terminal) => terminal.id === this._shownTerminal);
    if (!shown) {
      this.hideTerminal();
      return;
    }
    this._refs.terminal.show(shown);
  }

  /** A terminal of the project, from the panel: its view draws it again from the output the daemon kept. */
  private showTerminal(terminalId: string): void {
    const terminal = allTerminals().find((each) => each.id === terminalId);
    if (!terminal) {
      return;
    }
    this.drawTerminal(terminal);
    const size = this._refs.terminal.size();
    daemon.send({ ...size, terminalId, type: 'attachTerminal' });
  }

  /** A terminal of the project's folder, with the number the daemon gives it, drawn as soon as it is open. */
  private openTerminal(): void {
    const session = findSession(this._sessionId);
    if (session?.kind !== 'code') {
      return;
    }
    this.showProblem('');
    this._openingTerminal = true;
    daemon.send({
      columns: NEW_TERMINAL_COLUMNS,
      cwd: session.workspace,
      host: session.host,
      rows: NEW_TERMINAL_ROWS,
      type: 'openTerminal',
    });
  }

  /** The terminal opens in its dialog over the session; the one shown before leaves. */
  private drawTerminal(terminal: Terminal): void {
    if (this._shownTerminal !== '' && this._shownTerminal !== terminal.id) {
      daemon.send({ terminalId: this._shownTerminal, type: 'detachTerminal' });
    }
    this._shownTerminal = terminal.id;
    this._refs.terminal.show(terminal);
  }

  /** Back to the conversation: this socket stops hearing the terminal, which keeps running for the next visit. */
  private hideTerminal(): void {
    if (this._shownTerminal !== '') {
      daemon.send({ terminalId: this._shownTerminal, type: 'detachTerminal' });
    }
    this._shownTerminal = '';
    this._openingTerminal = false;
    this._refs.terminal.hide();
  }

  /** The terminal's own close button: its shell ends, and the list takes it out. */
  private closeShownTerminal(): void {
    const terminalId = this._shownTerminal;
    this.hideTerminal();
    daemon.send({ terminalId, type: 'closeTerminal' });
  }

  private typeInTerminal(data: string): void {
    if (this._shownTerminal === '') {
      return;
    }
    daemon.send({ data, terminalId: this._shownTerminal, type: 'terminalInput' });
  }

  private resizeTerminal(size: TerminalSize): void {
    if (this._shownTerminal === '' || size.columns === 0) {
      return;
    }
    daemon.send({ ...size, terminalId: this._shownTerminal, type: 'terminalResize' });
  }

  private receive(message: ServerMessage): void {
    this.handle(message.type, message);
  }

  private handle<Type extends keyof ServerMessageMap>(type: Type, message: ServerMessageMap[Type]): void {
    this._handlers[type]?.(message);
  }

  private forThisSession(sessionId: string, action: () => void): void {
    if (sessionId !== this._sessionId) {
      return;
    }
    action();
  }

  /** Back online after the daemon went away: the page asks for what the session recorded meanwhile, and goes on
   * where the person was reading. */
  private reopen(online: boolean): void {
    // A terminal this page was opening when the socket went away never came: the next one is another page's.
    this._openingTerminal = false;
    if (!online) {
      return;
    }
    this._opened = false;
    this.openHistory();
    if (this._shownTerminal !== '') {
      this.showTerminal(this._shownTerminal);
    }
  }

  /** Asks for the end of the conversation; a page that holds some of the session asks for what came after it. */
  private openHistory(): void {
    const held = this._keptBack + this._events.length;
    daemon.send({
      events: HISTORY_EVENTS,
      sessionId: this._sessionId,
      type: 'openSession',
      ...(held === 0 ? {} : { since: held }),
    });
  }

  private send(draft: PromptDraft): void {
    this.showProblem('');
    daemon.send({
      sessionId: this._sessionId,
      text: draft.text,
      type: 'sendPrompt',
      ...(draft.pictures.length === 0 ? {} : { pictures: draft.pictures }),
    });
  }

  /** A new session with this one's settings: the app opens it once the daemon made it. */
  private startNew(): void {
    const session = findSession(this._sessionId);
    if (!session) {
      return;
    }
    this.showProblem('');
    daemon.send(sameSettings(session));
  }

  /** A question beside the session: its answer shows above the composer, never in the conversation. */
  private peek(question: string): void {
    this._peeks += 1;
    // The daemon answers a peek by its id alone: a page's own id has to be one no other page's guesses.
    const peekId = `${this._sessionId}-${this._peeks}-${crypto.randomUUID()}`;
    this._refs.peek.ask(peekId, question);
    daemon.send({ peekId, question, sessionId: this._sessionId, type: 'peek' });
  }

  /** /todo: the text joins the todos of the folder this session works in, which the daemon lists back. */
  private addTodo(text: string): void {
    this.showProblem('');
    const session = findSession(this._sessionId);
    const project = session?.kind === 'code' ? projectAt(session.workspace, session.host) : undefined;
    if (!project) {
      this.showProblem(NO_PROJECT);
      return;
    }
    addProjectTodo(project.id, text);
    toast({ heading: `Todo added to ${project.name}`, tone: 'ok' });
  }

  private applyEvent(event: SessionEvent): void {
    this._events.push(event);
    this._refs.conversation.apply(event);
    if (PLAN_EVENTS.has(event.kind)) {
      this._refs.plan.show(currentPlan(this._events));
    }
    if (SUBAGENT_EVENTS.has(event.kind)) {
      this._refs.panel.agents.set(agentsOf(this._events));
      return;
    }
    // A summary empties the context: its count comes back with the next reply.
    if (event.kind === 'contextCompacted') {
      this.showUsage(0);
      return;
    }
    if (event.kind !== 'assistantMessage' || !event.usage) {
      return;
    }
    this.showUsage(event.usage.promptTokens + event.usage.completionTokens);
    this.showSpeed(event);
  }

  private showSpeed(reply: SessionEvent | undefined): void {
    this._refs.panel.speed.set(speedLine(reply?.kind === 'assistantMessage' ? reply.usage : undefined));
  }

  private showUsage(used: number): void {
    this._used = used;
    this._refs.panel.used.set(used);
    this.renderDaemon();
  }

  /** Deletes the session once the person confirms, its conversation for good; a new one of its kind shows then. */
  private async deleteSession(): Promise<void> {
    const session = findSession(this._sessionId);
    if (!session) {
      return;
    }
    if (!(await deleteSession(session))) {
      return;
    }
    navigate(newPathOf(session.kind));
  }

  /** What the daemon refused, over the composer: nothing once a new message goes. */
  private showProblem(text: string): void {
    // A refusal ends whatever this page was waiting for, a terminal it asked to open included.
    if (text !== '') {
      this._openingTerminal = false;
    }
    this._refs.problem.description.set(text);
    this._refs.problem.hidden = text === '';
  }

  /** Shows or hides the side panel, for this page and the next ones. */
  private showPanel(open: boolean): void {
    setSidePanelOpen(open);
    this.renderPanel(open);
  }

  /** Closed, the panel leaves its room to the conversation; its button offers the other way. */
  private renderPanel(open: boolean): void {
    this.reflect(PANEL_CLOSED, !open);
    this._refs.panelToggle.icon.set(open ? 'panel-close' : 'panel-open');
    this._refs.panelToggle.hint.set(open ? 'Hide the side panel' : 'Show the side panel');
  }

  private renderSummary(): void {
    const session = findSession(this._sessionId);
    this._refs.title.value.set(session?.title ?? NOT_FOUND);
    this._refs.title.fixed.set(session === undefined);
    this.renderDelete(session);
    this._refs.status.hidden = session === undefined;
    this._refs.status.showStatus(SESSION_STATUS_LOOKS[session ? liveStatus(session) : 'idle']);
    this._refs.conversation.queued.set(session?.queued ?? []);
    this._refs.conversation.queuedNow.set(session?.queuedNow ?? false);
    this._refs.panel.jobs.set(session?.jobs ?? []);
    const pins = session?.pins ?? [];
    this._refs.panel.pins.set(pins);
    this._refs.conversation.pinned.set(pins.map((pin) => pin.id));
    const kind = session?.kind ?? 'code';
    this._refs.place.hidden = kind === 'chat';
    // A chat works in no folder: it has nothing to review.
    this._refs.reviewOpen.hidden = session === undefined || kind === 'chat';
    this._refs.path.textContent = session ? sessionPlace(session) : '';
    this._refs.conversation.workspace.set(session?.workspace ?? '');
    this._refs.composer.kind.set(kind);
    this._refs.composer.commands.set([...SESSION_COMMANDS[kind]]);
    this.renderTerminals();
    this.renderSkills();
    this.renderLinkedTodos();
    this._refs.composer.mode.set(session?.mode ?? 'auto');
    this._refs.composer.thinking.set(session?.thinking ?? 'full');
    this._refs.composer.preset.set(session?.preset ?? '');
    this._refs.composer.concise.set(session?.concise ?? false);
    this.renderDaemon();
    const live = session ? liveStatus(session) : 'idle';
    this._refs.composer.working.set(live !== 'idle');
    this._refs.composer.background.set((session?.subagents ?? 0) > 0);
    this._refs.plan.working.set((session?.status ?? 'idle') !== 'idle');
    this._refs.panel.status.set(session?.status ?? 'idle');
    this._refs.arcade.status.set(live);
    this.noteSeen();
    document.title = session ? `${session.title} · swcode` : 'swcode';
  }

  /** The delete button: none without a session, held while it works, which the daemon would refuse to delete. */
  private renderDelete(session: SessionSummary | undefined): void {
    const remove = this._refs.remove;
    remove.hidden = session === undefined;
    if (session === undefined) {
      return;
    }
    remove.disabled.set(!canDeleteSession(session));
    remove.hint.set(deleteHintOf(session));
  }

  /**
   * The skills the session's project has on, the composer's / commands: read on its host once the session opens, a
   * chat has none.
   */
  private renderSkills(): void {
    const session = findSession(this._sessionId);
    const project = session?.kind === 'code' ? projectAt(session.workspace, session.host) : undefined;
    this._refs.composer.skills.set(project ? skillsOn(foundSkills(project.id), project) : []);
    if (!project || project.id === this._skillsOf) {
      return;
    }
    this._skillsOf = project.id;
    void readProjectSkills(project.id);
  }

  /** The project's todos this session was pinned to, in the order the project lists them. */
  private renderLinkedTodos(): void {
    const session = findSession(this._sessionId);
    const project = session?.kind === 'code' ? projectAt(session.workspace, session.host) : undefined;
    this._refs.todos.todos.set(project?.todos.filter((todo) => todo.sessionId === this._sessionId) ?? []);
  }

  /** The session in view is seen as it is now, while the window shows it: its icon in the navigation empties. */
  private noteSeen(): void {
    const session = findSession(this._sessionId);
    if (!session || document.visibilityState !== 'visible') {
      return;
    }
    markSeen(session);
  }

  /** What the session's main model is, as its llm-server says now: whether it sees, how much context it holds. */
  private renderDaemon(): void {
    const state = daemonState();
    const preset = presetOf(presetChoices(), findSession(this._sessionId)?.preset);
    const main = presetStatusOf(state.kind === 'ready' ? state.status : undefined, preset);
    this._refs.panel.daemon.set(state);
    this._refs.panel.main.set(main);
    this._refs.composer.seesPictures.set(main?.vision ?? false);
    this._refs.composer.context.set(contextText(this._used, main?.contextTokens));
  }
}

SessionPage.register();
