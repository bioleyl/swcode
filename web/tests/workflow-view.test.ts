import { describe, expect, it } from 'vitest';

import { flowOf, graphProblem } from '../src/lib/workflow-view';

import type { WorkflowEdge, WorkflowStep } from '../../shared/protocol/workflow.schema';

const step = (id: string, name: string, exits: WorkflowStep['exits']): WorkflowStep => ({
  exits,
  id,
  kind: 'step',
  name,
  position: { x: 0, y: 0 },
  prompt: 'Do the work.',
  role: '',
});

describe('the workflow graph', () => {
  it('keeps every edge on the output it leaves by', () => {
    const steps = [
      step('qa', 'QA', [
        { id: 'fix', label: 'Fix needed', when: 'it fails' },
        { id: 'pass', label: 'Passed', when: 'it passes' },
      ]),
      step('dev', 'Dev', []),
      step('po', 'PO', []),
    ];
    const edges: WorkflowEdge[] = [
      { from: 'qa', id: 'e1', port: 'fix', to: 'dev' },
      { from: 'qa', id: 'e2', port: 'pass', to: 'po' },
    ];
    const graph = flowOf(steps, edges);
    expect(graph.edges).toEqual([
      { from: 'qa', id: 'e1', port: 'fix', to: 'dev' },
      { from: 'qa', id: 'e2', port: 'pass', to: 'po' },
    ]);
    expect(graph.nodes.find((node) => node.id === 'qa')?.ports).toEqual([
      { id: 'fix', label: 'Fix needed' },
      { id: 'pass', label: 'Passed' },
    ]);
  });
});

describe('validating a workflow', () => {
  const start: WorkflowStep = {
    exits: [],
    id: 'start',
    kind: 'start',
    name: 'Start',
    position: { x: 0, y: 0 },
    prompt: '',
    role: '',
  };
  const end: WorkflowStep = {
    exits: [],
    id: 'end',
    kind: 'end',
    name: 'End',
    position: { x: 0, y: 0 },
    prompt: '',
    role: '',
  };

  it('refuses an output with no edge', () => {
    const qa = step('qa', 'QA', [
      { id: 'fix', label: 'Fix needed', when: 'it fails' },
      { id: 'pass', label: 'Passed', when: 'it passes' },
    ]);
    const dev = step('dev', 'Dev', []);
    const problem = graphProblem(
      [start, qa, dev, end],
      [
        { from: 'start', id: 's', to: 'qa' },
        { from: 'qa', id: 'e1', port: 'fix', to: 'dev' },
        { from: 'dev', id: 'f', to: 'end' },
      ]
    );
    expect(problem).toContain('The output pass of QA has no edge');
  });

  it('refuses a step that goes several ways without outputs', () => {
    const branch = step('branch', 'Branch', []);
    const left = step('left', 'Left', []);
    const right = step('right', 'Right', []);
    const problem = graphProblem(
      [start, branch, left, right, end],
      [
        { from: 'start', id: 's', to: 'branch' },
        { from: 'branch', id: 'e1', to: 'left' },
        { from: 'branch', id: 'e2', to: 'right' },
        { from: 'left', id: 'e3', to: 'end' },
        { from: 'right', id: 'e4', to: 'end' },
      ]
    );
    expect(problem).toContain('goes several ways: give it outputs');
  });

  it('needs a Start and an End', () => {
    expect(graphProblem([step('a', 'A', [])], [])).toBe('A workflow needs one Start and one End.');
  });
});
