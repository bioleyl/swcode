import { describe, expect, it } from 'vitest';

import { decisionMapOf } from '../src/lib/decision-map';

import type { SessionEvent } from '../../shared/protocol/session.schema';

const AT = '2026-11-18T14:32:00.000Z';

const reply = (id: string, text: string): SessionEvent => ({
  at: AT,
  id,
  kind: 'assistantMessage',
  text,
  toolCalls: [],
});

const decided = (callId: string, chosen: string, options: string[], why: string): SessionEvent[] => [
  { at: AT, callId, kind: 'toolCall', summary: `${chosen}`, tool: 'decision' },
  {
    at: AT,
    callId,
    display: { chosen, kind: 'decision', options, question: `Which ${chosen}?`, why },
    kind: 'toolFinished',
    meta: chosen,
    ok: true,
    output: '',
  },
];

describe('the decisions of a session', () => {
  it('chains the recorded choices, the first as the start', () => {
    const map = decisionMapOf([
      ...decided('c1', 'React', ['React', 'Vue'], 'The project already uses it.'),
      ...decided('c2', 'Vite', ['Vite', 'Webpack'], 'It is the one the app runs on.'),
    ]);
    expect(map.decisions.map((decision) => decision.kind)).toEqual(['decision', 'decision']);
    expect(map.decisions[0]?.label).toBe('Which React?');
    expect(map.decisions[0]?.line).toBe('Chosen · React');
    expect(map.decisions[0]?.text).toContain('Options: React, Vue');
    expect(map.decisions[0]?.text).toContain('Why: The project already uses it.');
    expect(map.graph.nodes[0]?.start).toBe(true);
    expect(map.graph.edges).toEqual([{ from: 'decision:c1', id: 'edge:decision:c2', to: 'decision:c2' }]);
  });

  it('leaves out the calls the model made, which are not decisions', () => {
    const map = decisionMapOf([
      reply('a1', 'Running it.'),
      { at: AT, callId: 'c9', kind: 'toolCall', summary: 'npm test', tool: 'bash' },
      { at: AT, callId: 'c9', kind: 'toolFinished', meta: 'exit code 0', ok: true, output: '' },
      ...decided('c1', 'React', ['React', 'Vue'], ''),
    ]);
    expect(map.decisions.map((decision) => decision.id)).toEqual(['decision:c1']);
    // A fork at the choice starts before the reply that made it, so the model decides again from what it knew then.
    expect(map.decisions[0]?.cut).toBe(0);
  });

  it('leaves out a choice whose call failed', () => {
    const map = decisionMapOf([
      reply('a1', 'Writing it.'),
      { at: AT, callId: 'c1', kind: 'toolCall', summary: 'Which framework?', tool: 'decision' },
      { at: AT, callId: 'c1', kind: 'toolFinished', meta: 'refused', ok: false, output: '' },
    ]);
    expect(map.decisions).toEqual([]);
  });

  it('has nothing for a session whose events hold no decision', () => {
    const map = decisionMapOf([{ at: AT, id: 'e1', kind: 'turnStarted' }]);
    expect(map.decisions).toEqual([]);
    expect(map.graph).toEqual({ edges: [], nodes: [] });
  });
});
