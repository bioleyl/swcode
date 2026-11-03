import { describe, expect, it } from 'vitest';

import { agentsOf } from '../src/lib/agents';
import { modelCards, shareText } from '../src/lib/model-cards';
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

  it('marks a subagent that did not finish its task as failed', () => {
    const events: SessionEvent[] = [
      started('a', 'Find it'),
      { agentId: 'a', at: AT, kind: 'subagentFinished', outcome: 'stopped' },
    ];
    expect(agentsOf(events)[0]?.state).toBe('failed');
  });
});

describe('modelCards', () => {
  const gmktec = { id: 'g', models: ['gpt-oss-120b'], name: 'gmktec', reachable: true, url: 'http://gmktec:8080' };
  const status = { llama: { presets: [], sources: [gmktec] }, workspace: '/w' };

  it('shows the models the daemon used, with their llama-server and their slots', () => {
    expect(
      modelCards(
        [
          { busy: [0], model: 'gpt-oss-120b', own: [0, 1], source: 'gmktec', total: 2 },
          { busy: [], model: 'qwen3-coder-30b', own: [2, 3], source: 'garfield', total: 4 },
        ],
        status,
        undefined
      )
    ).toEqual([
      { busy: [0], name: 'gpt-oss-120b', own: [0, 1], source: 'gmktec', total: 2 },
      { busy: [], name: 'qwen3-coder-30b', own: [2, 3], source: 'garfield', total: 4 },
    ]);
  });

  it('names the one model of a single-model server from what its source serves', () => {
    expect(
      modelCards([{ busy: [0], model: '', own: [0], source: 'gmktec', total: 1 }], status, undefined)
    ).toEqual([{ busy: [0], name: 'gpt-oss-120b', own: [0], source: 'gmktec', total: 1 }]);
  });

  it('shows the session’s main model before any request, and nothing when its llama-server does not answer', () => {
    expect(modelCards([], status, { id: 'default', model: 'gpt-oss-120b' })).toEqual([
      { busy: [], name: 'gpt-oss-120b', own: [], total: undefined },
    ]);
    expect(modelCards([], status, { id: 'default' })).toEqual([]);
  });
});

describe('shareText', () => {
  it('says which of the model’s slots this daemon uses, numbered from 1', () => {
    expect(shareText([0, 1], 4)).toBe('Its share: slots 1–2 of 4');
    expect(shareText([2, 3], 4)).toBe('Its share: slots 3–4 of 4');
    expect(shareText([1], 2)).toBe('Its share: slot 2 of 2');
  });

  it('says nothing when the daemon has all the slots', () => {
    expect(shareText([0, 1, 2, 3], 4)).toBeUndefined();
    expect(shareText([0], 1)).toBeUndefined();
  });
});

describe('modelName', () => {
  it('names a model llama-server knows by its file the way people would', () => {
    expect(modelName('/home/syware/models/qwen38/Qwen3.8-Flash-Next-IQ3_S-00001-of-00002.gguf')).toBe(
      'Qwen3.8-Flash-Next-IQ3_S'
    );
    expect(modelName('gpt-oss-120b')).toBe('gpt-oss-120b');
  });
});
