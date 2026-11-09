import { css } from '@jadis/core';
import { JlAlert, JlBadge, JlButton, JlIcon, toast } from '@syware/jlib';

import { ScElement } from '../base/sc-element';
import { agentsOf } from '../lib/agents';
import { SESSION_COMMANDS } from '../lib/commands';
import { contextText } from '../lib/format';
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
import { ScSidePanel } from '../organisms/sc-side-panel';
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
import { setSidePanelOpen, sidePanelOpen } from '../services/side-panel';
import { BASE_STYLES } from '../styles/shared';
import styles from './session-page.css?inline';

import type { ClientMessage, ServerMessage } from '../../../shared/protocol/messages.schema';
import type { SessionEvent, SessionEventKind, SessionSummary } from '../../../shared/protocol/session.schema';
import type { PromptDraft } from '../types/composer.types';
import type { ServerHandlers, ServerMessageMap } from '../types/page.types';

const NOT_FOUND = 'This session is not on the daemon.';
const NO_PROJECT = 'This session works in no project: a todo needs one.';
const VISIBILITY_CHANGE = 'visibilitychange';
// On the host while the side panel is hidden.
const PANEL_CLOSED = 'panel-closed';
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
  // Whether this page has its history yet: the next answer is a page of older events, not the conversation itself.
  private _opened = false;

  readonly route = this.useAttributes({
    id: (id) => {
      this._sessionId = id ?? '';
      this._refs.conversation.session.set(this._sessionId);
    },
  });

  private readonly _handlers: ServerHandlers = {
    assistantDelta: (message) =>
      this.forThisSession(message.sessionId, () => this._refs.conversation.delta(message.messageId, message.text)),
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
    sessionEvent: (message) => this.forThisSession(message.sessionId, () => this.applyEvent(message.event)),
    // The end of the conversation, or a page of its older events: the first draws the conversation again, the other
    // is added above what is shown.
    sessionHistory: (message) =>
      this.forThisSession(message.sessionId, () => {
        if (this._opened) {
          this._events = [...message.events, ...this._events];
          this._refs.conversation.addOlder(message.events, message.keptBack);
        } else {
          this._opened = true;
          this._events = [...message.events];
          this._refs.conversation.load(message.events, message.keptBack, message.streaming);
        }
        this._refs.panel.agents.set(agentsOf(this._events));
        this._refs.plan.show(currentPlan(this._events));
        this.showUsage(usedTokens(this._events));
      }),
    toolCallWriting: (message) =>
      this.forThisSession(message.sessionId, () => this._refs.conversation.writingCall(message.tool)),
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
    status: ref(JlBadge),
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
            <JlButton class="delete" icon="trash" variant="ghost" size="sm" hidden />
            <JlButton class="panel-toggle" variant="ghost" size="sm" />
          </header>
          <ScConversation />
          <JlAlert tone="danger" hidden />
          <ScPlan hidden />
          <ScPeek hidden />
          <ScSessionTodos hidden />
          <ScComposer />
        </div>
        <ScSidePanel />
        <ScArcade />
      </>
    );
  }

  templateCss(): string {
    return css`${BASE_STYLES}${styles}`;
  }

  onConnect(): void {
    this._opened = false;
    this._events = [];
    this.watchDaemon();
    this.wireConversation();
    this.wireComposer();
    this.wireFrame();
    this.renderSummary();
    this.renderDaemon();
    this.openHistory();
    this._refs.composer.focusText();
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
    conversation.events.register('older', (older) =>
      daemon.send({ events: HISTORY_EVENTS, keptBack: older, sessionId: this._sessionId, type: 'olderEvents' })
    );
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
    this._refs.panelToggle.events.register('press', () => this.showPanel(!sidePanelOpen()));
    this._refs.panel.events.register('stopJob', (jobId) =>
      daemon.send({ jobId, sessionId: this._sessionId, type: 'stopJob' })
    );
    this.renderPanel(sidePanelOpen());
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

  /** Back online after the daemon went away: the conversation is drawn again from what the daemon kept, so the
   * older pages the person had scrolled to come with it. */
  private reopen(online: boolean): void {
    if (!online) {
      return;
    }
    this._opened = false;
    this.openHistory();
  }

  /** Asks for the end of the conversation, and for as much before it as the window already shows. */
  private openHistory(): void {
    daemon.send({
      events: HISTORY_EVENTS + this._events.length,
      sessionId: this._sessionId,
      type: 'openSession',
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
    const kind = session?.kind ?? 'code';
    this._refs.place.hidden = kind === 'chat';
    this._refs.path.textContent = session ? sessionPlace(session) : '';
    this._refs.conversation.workspace.set(session?.workspace ?? '');
    this._refs.composer.kind.set(kind);
    this._refs.composer.commands.set([...SESSION_COMMANDS[kind]]);
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
