import { toolLabel } from './tool-labels';

import type { SessionEvent, ToolName } from '../../../shared/protocol/session.schema';
import type { AgentView } from '../types/conversation.types';

/** A subagent's call in a few words: "Grep "writeFileSync"". */
export const lineText = (tool: ToolName | undefined, summary: string): string =>
  `${toolLabel(tool).name} ${summary}`;

/** How one event changes the subagents: one starts, does something, or ends. */
function applyEvent(agents: Map<string, AgentView>, event: SessionEvent): void {
  if (event.kind === 'subagentStarted') {
    agents.set(event.agentId, {
      activity: '',
      description: event.description,
      id: event.agentId,
      model: event.model,
      state: 'working',
    });
    return;
  }
  const agent = 'agentId' in event ? agents.get(event.agentId) : undefined;
  if (!agent) {
    return;
  }
  if (event.kind === 'subagentActivity') {
    agents.set(agent.id, { ...agent, activity: lineText(event.tool, event.summary) });
    return;
  }
  if (event.kind === 'subagentFinished') {
    agents.set(agent.id, { ...agent, state: event.outcome === 'done' ? 'done' : 'failed' });
  }
}

/** The session's subagents, from its events: the working ones first, then the latest that finished. */
export function agentsOf(events: readonly SessionEvent[]): AgentView[] {
  const agents = new Map<string, AgentView>();
  for (const event of events) {
    applyEvent(agents, event);
  }
  const all = [...agents.values()].reverse();
  return [
    ...all.filter((agent) => agent.state === 'working'),
    ...all.filter((agent) => agent.state !== 'working'),
  ];
}
