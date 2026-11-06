import { createElement, css } from '@jadis/core';
import { JlButton, JlProgress } from '@syware/jlib';

import { ScElement } from '../base/sc-element';
import { daemonLine, hostOf } from '../lib/daemon-lines';
import { tokens } from '../lib/format';
import { BASE_STYLES } from '../styles/shared';
import styles from './sc-side-panel.css?inline';

import type { PresetStatus, SourceStatus } from '../../../shared/protocol/models.schema';
import type { SessionStatus, SessionSummary } from '../../../shared/protocol/session.schema';
import type { AgentView } from '../types/conversation.types';
import type { DaemonState } from '../types/daemon.types';

const ACTIVITY: Readonly<Record<SessionStatus, string>> = {
  idle: 'Waiting for your message',
  waiting: 'Waiting for you',
  working: 'Working',
};

// The list stays short: the working subagents, then the latest that finished.
const MAX_AGENTS_SHOWN = 4;

// What a subagent shows before its first call ends.
const STARTING = 'Starting';

const agentNode = (agent: AgentView): Node => (
  <div class="agent" data-state={agent.state} title={agent.description}>
    <span class="dot" aria-hidden="true" />
    <div class="who">
      <p class="heading">
        <span class="name">{agent.description}</span>
        <span class="model">{agent.model ?? ''}</span>
      </p>
      <p class="activity">{agent.activity || STARTING}</p>
    </div>
  </div>
);

const CONNECTED = 'Connected';
const UNREACHABLE = 'Unreachable';

/** An llm-server the agents ask: its name, whether it answers, and its address under them. */
const serverNode = (source: SourceStatus): Node => (
  <div class="server" data-reachable={String(source.reachable)} title={source.url}>
    <span class="dot" aria-hidden="true" />
    <div class="who">
      <p class="heading">
        <span class="name">{source.name}</span>
        <span class="state">{source.reachable ? CONNECTED : UNREACHABLE}</span>
      </p>
      <p class="address">{hostOf(source.url)}</p>
    </div>
  </div>
);

type SessionJob = SessionSummary['jobs'][number];

/**
 * Beside a session on a wide screen: the agents at work, the commands they left running, the llm-servers and
 * whether they answer, how full the context is, and at the bottom which build of swcode runs. `stopJob` asks to stop
 * a command.
 */
export class ScSidePanel extends ScElement {
  static readonly selector = 'sc-side-panel';

  readonly events = this.useEvents<{ stopJob: string }>();

  readonly status = this.useChange<SessionStatus>(
    'idle',
    (value) => {
      this._refs.activity.textContent = ACTIVITY[value];
      this.reflect('status', value);
    },
    { immediate: true }
  );
  readonly daemon = this.useChange<DaemonState>(
    { kind: 'unknown' },
    (state) => {
      this.renderServers(state);
      this.render();
    },
    { immediate: true }
  );
  readonly agents = this.useChange<AgentView[]>([], (agents) =>
    this._refs.subagents.replaceChildren(...agents.slice(0, MAX_AGENTS_SHOWN).map(agentNode))
  );
  /** The prompt tokens of the last reply: the conversation so far. */
  readonly used = this.useChange(0, () => this.render());
  /** The session's main model, as its llm-server says now: the context it holds, and the model before any request. */
  readonly main = this.useChange<PresetStatus | undefined>(undefined, () => this.render());
  /** The commands the agent left running in the background. */
  readonly jobs = this.useChange<SessionJob[]>([], (jobs) => this.renderJobs(jobs));

  private readonly _refs = this.useRefs((ref) => ({
    activity: ref('.main .activity'),
    context: ref(JlProgress),
    jobs: ref('.jobs'),
    jobsSection: ref('.background'),
    of: ref('.of'),
    servers: ref('.servers'),
    subagents: ref('.subagents'),
    used: ref('.used'),
    version: ref('.version'),
  }));

  templateHtml(): Node {
    return (
      <>
        <section>
          <h2 class="label">Agents</h2>
          <div class="agent main">
            <span class="dot" aria-hidden="true" />
            <div class="who">
              <p class="heading">
                <span class="name">main</span>
              </p>
              <p class="activity" />
            </div>
          </div>
          <div class="subagents" />
        </section>
        <section class="background" hidden>
          <h2 class="label">Running in the background</h2>
          <div class="jobs" />
        </section>
        <section>
          <h2 class="label">llm-server</h2>
          <div class="servers" />
        </section>
        <section>
          <h2 class="label">Context</h2>
          <p>
            <strong class="used" /> <span class="of" />
          </p>
          <JlProgress size="sm" />
        </section>
        <p class="version" />
      </>
    );
  }

  templateCss(): string {
    return css`${BASE_STYLES}${styles}`;
  }

  /** A command, and its stop button: a button keeps its clicks to itself, so each says which command it stops. */
  private jobNode(job: SessionJob): Node {
    const stop = createElement(JlButton, {
      attrs: { class: 'stop' },
      props: { hint: `Stop ${job.command}`, icon: 'stop', size: 'sm', variant: 'ghost' },
    });
    stop.events.register('press', () => this.events.emit('stopJob', job.id));
    return (
      <div class="job" title={job.command}>
        <span class="dot" aria-hidden="true" />
        <code class="command">{job.command}</code>
        {stop}
      </div>
    );
  }

  private renderJobs(jobs: readonly SessionJob[]): void {
    this._refs.jobsSection.hidden = jobs.length === 0;
    this._refs.jobs.replaceChildren(...jobs.map((job) => this.jobNode(job)));
  }

  /** Each llm-server and whether it answers; while the daemon does not, what the app knows of it. */
  private renderServers(state: DaemonState): void {
    const servers =
      state.kind === 'ready'
        ? state.status.llama.sources.map(serverNode)
        : [<p class="servers-note">{daemonLine(state).text}</p>];
    this._refs.servers.replaceChildren(...servers);
  }

  private render(): void {
    const state = this.daemon.get();
    const status = state.kind === 'ready' ? state.status : undefined;
    const used = this.used.get();
    const capacity = this.main.get()?.contextTokens;
    this._refs.used.textContent = tokens(used);
    this._refs.of.textContent = capacity ? `of ${tokens(capacity)} tokens` : 'tokens';
    this._refs.context.max.set(capacity ?? 1);
    this._refs.context.value.set(capacity ? Math.min(used, capacity) : 0);
    this._refs.version.textContent = status?.version === undefined ? '' : `swcode ${status.version}`;
  }
}

ScSidePanel.register();
