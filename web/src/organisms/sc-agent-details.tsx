import { css } from '@jadis/core';
import { JlBadge, JlDrawer, JlIcon } from '@syware/jlib';

import { ScElement } from '../base/sc-element';
import { AGENT_STATE_LOOKS } from '../lib/agents';
import { toolLabel } from '../lib/tool-labels';
import { BASE_STYLES } from '../styles/shared';
import styles from './sc-agent-details.css?inline';

import type { AgentView, SubagentLine } from '../types/conversation.types';

const CALL_ICON_SIZE = 14;
const TASK_LABEL = 'Task';
const CALLS_LABEL = 'Calls';
const NO_CALLS = 'It has made no calls yet.';

/** One call the subagent made: its tool, what it did, and the few words of its result. */
const callNode = (call: SubagentLine): Node => {
  const label = toolLabel(call.tool);
  return (
    <div class="call" data-ok={String(call.ok)}>
      <JlIcon name={label.icon} size={CALL_ICON_SIZE} />
      <span class="call-name">{label.name}</span>
      <span class="call-summary" title={call.summary}>
        {call.summary}
      </span>
      <span class="call-meta">{call.meta ?? ''}</span>
    </div>
  );
};

/**
 * A subagent in a drawer beside the session: the whole task it was given, its model, its state, and every call it
 * made; it follows the agent while it works.
 */
export class ScAgentDetails extends ScElement {
  static readonly selector = 'sc-agent-details';

  /** The subagent the drawer shows; none closes it. */
  readonly agent = this.useChange<AgentView | undefined>(undefined, (agent) => this.render(agent));

  private readonly _refs = this.useRefs((ref) => ({
    badge: ref(JlBadge),
    calls: ref('.calls'),
    callsEmpty: ref('.calls-empty'),
    drawer: ref(JlDrawer),
    prompt: ref('.prompt'),
    task: ref('.task'),
  }));

  templateHtml(): Node {
    return (
      <JlDrawer size="md">
        <JlBadge class="state" slot="actions" />
        <section class="task" hidden>
          <h2 class="label">{TASK_LABEL}</h2>
          <p class="prompt" />
        </section>
        <section class="calls-section">
          <h2 class="label">{CALLS_LABEL}</h2>
          <p class="calls-empty">{NO_CALLS}</p>
          <div class="calls" />
        </section>
      </JlDrawer>
    );
  }

  templateCss(): string {
    return css`${BASE_STYLES}${styles}`;
  }

  /** The subagent whose details open, at once. */
  show(agent: AgentView): void {
    this.agent.set(agent);
    this._refs.drawer.show();
  }

  private render(agent: AgentView | undefined): void {
    if (!agent) {
      this._refs.drawer.close();
      return;
    }
    this._refs.drawer.heading.set(agent.description);
    this._refs.drawer.description.set(agent.model ?? '');
    this._refs.badge.showStatus(AGENT_STATE_LOOKS[agent.state]);
    this._refs.task.hidden = agent.prompt === '';
    this._refs.prompt.textContent = agent.prompt;
    this._refs.callsEmpty.hidden = agent.calls.length > 0;
    this._refs.calls.replaceChildren(...agent.calls.map(callNode));
  }
}

ScAgentDetails.register();
