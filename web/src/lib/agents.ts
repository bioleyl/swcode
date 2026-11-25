import { toolLabel } from './tool-labels';

import type { StatusLook } from '@syware/jlib';
import type { SessionEvent, ToolName } from '../../../shared/protocol/session.schema';
import type { AgentState, AgentView, SubagentLine } from '../types/conversation.types';

/** A subagent's call in a few words: "Grep "writeFileSync"". */
export const lineText = (tool: ToolName | undefined, summary: string): string =>
  `${toolLabel(tool).name} ${summary}`;

/** How each state of a subagent shows, the same in every view: a new state does not compile until it is here. */
export const AGENT_STATE_LOOKS: Readonly<Record<AgentState, StatusLook>> = {
  done: {
    hint: 'Its task ended, and its report went to the main agent.',
    icon: 'check',
    label: 'Done',
    tone: 'neutral',
  },
  failed: {
    hint: 'Its task ended short: its report says why.',
    icon: 'x',
    label: 'Failed',
    tone: 'danger',
  },
  waiting: {
    hint: 'As many subagents work at once as the preset allows: this one starts when one of them reports.',
    icon: 'circle',
    label: 'Waiting its turn',
    tone: 'info',
  },
  working: { hint: 'Its task runs now, beside the main agent.', icon: 'loader', label: 'Working', tone: 'accent' },
};

/** What a subagent does now, in a few words: its latest call, or its state before it made one. */
export const agentLine = (agent: AgentView): string => {
  const last = agent.calls.at(-1);
  return last ? lineText(last.tool, last.summary) : AGENT_STATE_LOOKS[agent.state].label;
};

/** A subagent that waits its turn or starts: one that waited keeps its place in the list, and what it was asked. */
const startedAgent = (
  event: Extract<SessionEvent, { kind: 'subagentQueued' | 'subagentStarted' }>,
  previous: AgentView | undefined
): AgentView => ({
  calls: previous?.calls ?? [],
  description: event.description,
  id: event.agentId,
  model: event.model,
  prompt: event.prompt ?? previous?.prompt ?? '',
  state: event.kind === 'subagentQueued' ? 'waiting' : 'working',
});

/** How one event changes the subagents: one waits its turn, starts, does something, or ends. */
function applyEvent(agents: Map<string, AgentView>, event: SessionEvent): void {
  if (event.kind === 'subagentQueued' || event.kind === 'subagentStarted') {
    agents.set(event.agentId, startedAgent(event, agents.get(event.agentId)));
    return;
  }
  const agent = 'agentId' in event ? agents.get(event.agentId) : undefined;
  if (!agent) {
    return;
  }
  if (event.kind === 'subagentActivity') {
    const call: SubagentLine = { meta: event.meta, ok: event.ok, summary: event.summary, tool: event.tool };
    agents.set(agent.id, { ...agent, calls: [...agent.calls, call] });
    return;
  }
  if (event.kind === 'subagentFinished') {
    agents.set(agent.id, { ...agent, state: event.outcome === 'done' ? 'done' : 'failed' });
  }
}

/** The session's subagents, from its events: the working ones first, then those waiting, then the latest that finished. */
export function agentsOf(events: readonly SessionEvent[]): AgentView[] {
  const agents = new Map<string, AgentView>();
  for (const event of events) {
    applyEvent(agents, event);
  }
  const all = [...agents.values()].reverse();
  return [
    ...all.filter((agent) => agent.state === 'working'),
    ...all.filter((agent) => agent.state === 'waiting'),
    ...all.filter((agent) => agent.state !== 'working' && agent.state !== 'waiting'),
  ];
}
