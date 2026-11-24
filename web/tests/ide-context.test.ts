import { describe, expect, it } from 'vitest';

import { contextBlock } from '../src/lib/ide-context';

import type { IdeContext, IdeDiagnostic, IdeSelection } from '../src/types/ide-context.types';

const CARET: IdeSelection = { endColumn: 1, endLine: 1, startColumn: 1, startLine: 1, text: '' };

const context = (parts: Partial<IdeContext> = {}): IdeContext => ({
  diagnostics: [],
  language: 'typescript',
  path: '/home/dev/app/src/main.ts',
  selection: { endColumn: 9, endLine: 3, startColumn: 1, startLine: 1, text: 'const a = 1;\nconst b = 2;' },
  ...parts,
});

const diagnostic = (parts: Partial<IdeDiagnostic> = {}): IdeDiagnostic => ({
  code: 'no-unused-vars',
  column: 7,
  line: 1,
  message: "'a' is never read.",
  severity: 'warning',
  source: 'eslint',
  ...parts,
});

describe('the IDE context block', () => {
  it('names the file, the language and the lines selected', () => {
    expect(contextBlock(context())).toBe(
      '```\nFile: /home/dev/app/src/main.ts\nSelection: 1:1-3:9\nSelected code (typescript):\nconst a = 1;\nconst b = 2;\n```'
    );
  });

  it('gives the caret a range of one place, with the empty selection', () => {
    expect(contextBlock(context({ selection: CARET }))).toContain('Selection: 1:1\nSelected code (typescript):\n');
  });

  it('lists the problems with their place, their severity and the rule that found them', () => {
    const block = contextBlock(context({ diagnostics: [diagnostic(), diagnostic({ code: '', source: '' })] }));
    expect(block).toContain(
      "Problems:\n1:7 warning 'a' is never read. (eslint/no-unused-vars)\n1:7 warning 'a' is never read."
    );
  });

  it('leaves the problems out when the file has none', () => {
    expect(contextBlock(context())).not.toContain('Problems:');
  });

  it('opens a fence longer than the longest run of backticks in the code', () => {
    const block = contextBlock(context({ selection: { ...CARET, text: 'const t = ```${x}```;' } }));
    expect(block.startsWith('````\n')).toBe(true);
    expect(block.endsWith('\n````')).toBe(true);
  });

  it('cuts a long selection to 200 lines', () => {
    const long = Array.from({ length: 240 }, (_, index) => `l${index}`).join('\n');
    const lines = contextBlock(context({ selection: { ...CARET, text: long } })).split('\n');
    expect(lines.filter((line) => line.startsWith('l'))).toHaveLength(200);
    expect(lines).toContain('l199');
  });

  it('cuts the problems to 30', () => {
    const many = Array.from({ length: 40 }, (_, index) => diagnostic({ line: index + 1 }));
    const block = contextBlock(context({ diagnostics: many }));
    expect(block.split('\n').filter((line) => line.includes('warning'))).toHaveLength(30);
  });
});
