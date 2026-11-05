import { css } from '@jadis/core';
import { confirmDialog, JlAlert, JlBadge, JlButton, JlIcon } from '@syware/jlib';

import { ScElement } from '../base/sc-element';
import { agentsOf } from '../lib/agents';
import { contextText } from '../lib/format';
import { placeText } from '../lib/hosts';
import { currentPlan } from '../lib/plan';
import { presetOf, presetStatusOf } from '../lib/presets';
import { SESSION_STATUS_LOOKS } from '../lib/session-status';
import { ScTitle } from '../molecules/sc-title';
import { ScArcade } from '../organisms/sc-arcade';
import { ScComposer } from '../organisms/sc-composer';
import { ScConversation } from '../organisms/sc-conversation';
import { ScPlan } from '../organisms/sc-plan';
import { ScSidePanel } from '../organisms/sc-side-panel';
import { daemon, daemonBus } from '../services/daemon';
import { daemonState, daemonStatusBus } from '../services/daemon-status';
import { HOME_PATH, navigate } from '../services/navigation';
import { presetChoices, presetsBus } from '../services/presets';
import { markSeen } from '../services/seen-sessions';
import { findSession, sessionListBus } from '../services/session-list';
import { setSidePanelOpen, sidePanelOpen } from '../services/side-panel';
import { BASE_STYLES } from '../styles/shared';
import styles from './session-page.css?inline';

import type { ServerMessage } from '../../../shared/protocol/messages.schema';
import type { SessionEvent, SessionEventKind } from '../../../shared/protocol/session.schema';
import type { PromptDraft } from '../types/composer.types';
import type { ServerHandlers, ServerMessageMap } from '../types/page.types';

const NOT_FOUND = 'This session is not on the daemon.';
const VISIBILITY_CHANGE = 'visibilitychange';
// On the host while the side panel is hidden.
const PANEL_CLOSED = 'panel-closed';
// The events that can change the plan above the composer: a new turn hides the last one, a plan call shows its own.
const PLAN_EVENTS: ReadonlySet<SessionEventKind> = new Set(['turnStarted', 'toolFinished']);
const SUBAGENT_EVENTS: ReadonlySet<SessionEventKind> = new Set([
  'subagentStarted',
  'subagentActivity',
  'subagentFinished',
]);

/** The prompt tokens of the last reply, plus its own: how much of the context the conversation fills. */
const usedTokens = (events: readonly SessionEvent[]): number => {
  const last = events.findLast((event) => event.kind === 'assistantMessage' && event.usage !== undefined);
  return last?.kind === 'assistantMessage' && last.usage
    ? last.usage.promptTokens + last.usage.completionTokens
    : 0;
};

/** One session: its conversation, the composer under it, and on a wide screen what the agent runs on. */
export class SessionPage extends ScElement {
  static readonly selector = 'session-page';

  private _sessionId = '';
  private _used = 0;
  // The session's events so far: the side panel lists its subagents from them.
  private _events: SessionEvent[] = [];

  readonly route = this.useAttributes({
    id: (id) => {
      this._sessionId = id ?? '';
      this._refs.conversation.session.set(this._sessionId);
    },
  });

  private readonly _handlers: ServerHandlers = {
    assistantDelta: (message) =>
      this.forThisSession(message.sessionId, () => this._refs.conversation.delta(message.messageId, message.text)),
    // Projects and folders come through the project list service and the project dialog.
    folders: () => undefined,
    hosts: () => undefined,
    // The presets come through the presets service.
    presets: () => undefined,
    problem: (message) => this.showProblem(message.message),
    projectCreated: () => undefined,
    projectRefused: () => undefined,
    projects: () => undefined,
    reasoningDelta: (message) =>
      this.forThisSession(message.sessionId, () =>
        this._refs.conversation.reasoning(message.messageId, message.text)
      ),
    sessionCreated: () => undefined,
    sessionEvent: (message) => this.forThisSession(message.sessionId, () => this.applyEvent(message.event)),
    sessionHistory: (message) =>
      this.forThisSession(message.sessionId, () => {
        this._events = [...message.events];
        this._refs.conversation.load(message.events, message.streaming);
        this._refs.panel.agents.set(agentsOf(this._events));
        this._refs.plan.show(currentPlan(this._events));
        this.showUsage(usedTokens(message.events));
      }),
    // The list itself comes through the session list service; the slots the app no longer shows.
    sessions: () => undefined,
    slots: () => undefined,
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
    plan: ref(ScPlan),
    problem: ref(JlAlert),
    remove: ref<JlButton>('.delete'),
    status: ref(JlBadge),
    title: ref(ScTitle),
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
            <JlButton class="delete" icon="trash" hint="Delete this session" variant="ghost" size="sm" />
            <JlButton class="panel-toggle" variant="ghost" size="sm" />
          </header>
          <ScConversation />
          <JlAlert tone="danger" hidden />
          <ScPlan hidden />
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
    const { composer, conversation } = this._refs;
    this.onBus(daemonBus, 'message', (message) => this.receive(message));
    this.onBus(daemonBus, 'online', (online) => this.reopen(online));
    this.onBus(sessionListBus, 'changed', () => this.renderSummary());
    this.on(document, VISIBILITY_CHANGE, () => this.noteSeen());
    this.onBus(daemonStatusBus, 'changed', () => this.renderDaemon());
    this.onBus(presetsBus, 'changed', () => this.renderDaemon());
    conversation.events.register('answer', ({ decision, requestId }) =>
      daemon.send({ decision, requestId, sessionId: this._sessionId, type: 'answerPermission' })
    );
    composer.events.register('send', (draft) => this.send(draft));
    conversation.events.register('question', ({ answers, requestId }) =>
      daemon.send({ answers, requestId, sessionId: this._sessionId, type: 'answerQuestion' })
    );
    conversation.events.register('cancelQueue', () =>
      daemon.send({ sessionId: this._sessionId, type: 'clearQueue' })
    );
    conversation.events.register('sendQueueNow', () =>
      daemon.send({ sessionId: this._sessionId, type: 'sendQueueNow' })
    );
    this._refs.title.events.register('rename', (title) =>
      daemon.send({ sessionId: this._sessionId, title, type: 'renameSession' })
    );
    this._refs.remove.events.register('press', () => void this.deleteSession());
    this._refs.panelToggle.events.register('press', () => this.showPanel(!sidePanelOpen()));
    this._refs.panel.events.register('stopJob', (jobId) =>
      daemon.send({ jobId, sessionId: this._sessionId, type: 'stopJob' })
    );
    this.renderPanel(sidePanelOpen());
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
    this.renderSummary();
    this.renderDaemon();
    daemon.send({ sessionId: this._sessionId, type: 'openSession' });
    composer.focusText();
  }

  private receive(message: ServerMessage): void {
    this.handle(message.type, message);
  }

  private handle<Type extends keyof ServerMessageMap>(type: Type, message: ServerMessageMap[Type]): void {
    this._handlers[type](message);
  }

  private forThisSession(sessionId: string, action: () => void): void {
    if (sessionId !== this._sessionId) {
      return;
    }
    action();
  }

  /** Back online after the daemon went away: the session is drawn again from what the daemon kept. */
  private reopen(online: boolean): void {
    if (!online) {
      return;
    }
    daemon.send({ sessionId: this._sessionId, type: 'openSession' });
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

  /** Deletes the session once the person confirms: its conversation goes for good. */
  private async deleteSession(): Promise<void> {
    const title = findSession(this._sessionId)?.title ?? '';
    const confirmed = await confirmDialog({
      confirmLabel: 'Delete',
      heading: `Delete “${title}”?`,
      text: 'Its conversation goes for good. Its project and files stay.',
      tone: 'danger',
    });
    if (!confirmed) {
      return;
    }
    daemon.send({ sessionId: this._sessionId, type: 'deleteSession' });
    navigate(HOME_PATH);
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
    this._refs.remove.hidden = session === undefined;
    this._refs.status.hidden = session === undefined;
    this._refs.status.showStatus(SESSION_STATUS_LOOKS[session?.status ?? 'idle']);
    this._refs.conversation.queued.set(session?.queued ?? []);
    this._refs.conversation.queuedNow.set(session?.queuedNow ?? false);
    this._refs.panel.jobs.set(session?.jobs ?? []);
    this._refs.path.textContent = session ? placeText(session.host, session.workspace) : '';
    this._refs.conversation.workspace.set(session?.workspace ?? '');
    this._refs.composer.mode.set(session?.mode ?? 'auto');
    this._refs.composer.thinking.set(session?.thinking ?? 'full');
    this._refs.composer.preset.set(session?.preset ?? '');
    this._refs.composer.concise.set(session?.concise ?? false);
    this.renderDaemon();
    this._refs.composer.working.set((session?.status ?? 'idle') !== 'idle');
    this._refs.plan.working.set((session?.status ?? 'idle') !== 'idle');
    this._refs.panel.status.set(session?.status ?? 'idle');
    this._refs.arcade.status.set(session?.status ?? 'idle');
    this.noteSeen();
    document.title = session ? `${session.title} · swcode` : 'swcode';
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
