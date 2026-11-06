import { describe, expect, it } from 'vitest';

import { agentsOf } from '../src/lib/agents';
import { modelName } from '../src/lib/model-name';

import type { SessionEvent } from '../../shared/protocol/session.schema';

const AT = '2026-10-23T12:00:00.000Z';

const started = (agentId: string, description: string): SessionEvent => ({
  agentId,
  at: AT,
  callId: `call-${agentId}`,
  description,
  kind: 'subagentStarted',
  model: 'qwen3-coder-30b',
});

describe('agentsOf', () => {
  it('follows each subagent from its start to its end, with its latest call', () => {
    const events: SessionEvent[] = [
      started('a', 'Find the store'),
      started('b', 'Find the tools'),
      { agentId: 'a', at: AT, kind: 'subagentActivity', ok: true, summary: '"writeFileSync"', tool: 'grep' },
      { agentId: 'a', at: AT, kind: 'subagentFinished', outcome: 'done' },
      { agentId: 'b', at: AT, kind: 'subagentActivity', ok: true, summary: 'src/tools.ts', tool: 'read' },
    ];
    expect(agentsOf(events)).toEqual([
      {
        activity: 'Read src/tools.ts',
        description: 'Find the tools',
        id: 'b',
        model: 'qwen3-coder-30b',
        state: 'working',
      },
      {
        activity: 'Grep "writeFileSync"',
        description: 'Find the store',
        id: 'a',
        model: 'qwen3-coder-30b',
        state: 'done',
      },
    ]);
  });

  it('lists a subagent waiting its turn after those at work, then at work once it starts', () => {
    const queued: SessionEvent = {
      agentId: 'b',
      at: AT,
      callId: 'call-b',
      description: 'Find the tools',
      kind: 'subagentQueued',
      model: 'qwen3-coder-30b',
    };
    const waiting = agentsOf([started('a', 'Find the store'), queued]);
    expect(waiting.map((agent) => [agent.id, agent.state])).toEqual([
      ['a', 'working'],
      ['b', 'waiting'],
    ]);
    const finished: SessionEvent = { agentId: 'a', at: AT, kind: 'subagentFinished', outcome: 'done' };
    const later = agentsOf([started('a', 'Find the store'), queued, finished, started('b', 'Find the tools')]);
    expect(later.map((agent) => [agent.id, agent.state])).toEqual([
      ['b', 'working'],
      ['a', 'done'],
    ]);
  });

  it('marks a subagent that did not finish its task as failed', () => {
    const events: SessionEvent[] = [
      started('a', 'Find it'),
      { agentId: 'a', at: AT, kind: 'subagentFinished', outcome: 'stopped' },
    ];
    expect(agentsOf(events)[0]?.state).toBe('failed');
  });
});

describe('modelName', () => {
  it('names a model llm-server knows by its file the way people would', () => {
    expect(modelName('/home/syware/models/qwen38/Qwen3.8-Flash-Next-IQ3_S-00001-of-00002.gguf')).toBe(
      'Qwen3.8-Flash-Next-IQ3_S'
    );
    expect(modelName('gpt-oss-120b')).toBe('gpt-oss-120b');
  });
});
