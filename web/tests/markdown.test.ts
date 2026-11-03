import { describe, expect, it } from 'vitest';

import { blocksOf } from '../src/lib/markdown';

describe('blocksOf, beyond paragraphs', () => {
  it('reads a list right under its introduction', () => {
    expect(blocksOf('I changed:\n- `a.ts`\n- the **tests**')).toEqual([
      { kind: 'paragraph', spans: [{ kind: 'text', text: 'I changed:' }] },
      {
        items: [
          [{ kind: 'code', text: 'a.ts' }],
          [
            { kind: 'text', text: 'the ' },
            { kind: 'strong', text: 'tests' },
          ],
        ],
        kind: 'list',
        ordered: false,
      },
    ]);
  });

  it('reads numbered lists and headings', () => {
    expect(blocksOf('## Steps\n1. Read\n2) Edit')).toEqual([
      { kind: 'heading', level: 2, spans: [{ kind: 'text', text: 'Steps' }] },
      {
        items: [[{ kind: 'text', text: 'Read' }], [{ kind: 'text', text: 'Edit' }]],
        kind: 'list',
        ordered: true,
      },
    ]);
  });

  it('keeps a dash inside a sentence as text', () => {
    expect(blocksOf('A - B')).toEqual([{ kind: 'paragraph', spans: [{ kind: 'text', text: 'A - B' }] }]);
  });
});

describe('blocksOf, tables', () => {
  const Table = [
    'The six modules in src/ (~1,760 LOC)',
    '| Module | What it does |',
    '|---|---|',
    '| jsx/ | Hand-written JSX runtime (jsx/jsxs/Fragment) |',
    '| router/ | Sniffs `HX-Request: true` to pick **onPartial** |',
  ].join('\n');

  it('reads a table right under its introduction, with inline code and bold in its cells', () => {
    expect(blocksOf(Table)).toEqual([
      { kind: 'paragraph', spans: [{ kind: 'text', text: 'The six modules in src/ (~1,760 LOC)' }] },
      {
        align: ['none', 'none'],
        header: [[{ kind: 'text', text: 'Module' }], [{ kind: 'text', text: 'What it does' }]],
        kind: 'table',
        rows: [
          [
            [{ kind: 'text', text: 'jsx/' }],
            [{ kind: 'text', text: 'Hand-written JSX runtime (jsx/jsxs/Fragment)' }],
          ],
          [
            [{ kind: 'text', text: 'router/' }],
            [
              { kind: 'text', text: 'Sniffs ' },
              { kind: 'code', text: 'HX-Request: true' },
              { kind: 'text', text: ' to pick ' },
              { kind: 'strong', text: 'onPartial' },
            ],
          ],
        ],
      },
    ]);
  });

  it('lines columns up by their colons, and fits every row to the header', () => {
    const [table] = blocksOf('a | b | c\n:-- | :-: | --:\n1 | 2\n1 | 2 | 3 | 4');
    expect(table).toMatchObject({ align: ['left', 'center', 'right'], kind: 'table' });
    expect(table?.kind === 'table' && table.rows.map((row) => row.length)).toEqual([3, 3]);
  });

  it('keeps a pipe that is escaped or in code inside its cell', () => {
    const [table] = blocksOf('| Pattern | Means |\n| --- | --- |\n| `a|b` | a \\| b |');
    expect(table?.kind === 'table' && table.rows[0]).toEqual([
      [{ kind: 'code', text: 'a|b' }],
      [{ kind: 'text', text: 'a | b' }],
    ]);
  });

  it('is no table without the line of dashes, nor a line of dashes alone', () => {
    expect(blocksOf('a | b\nc | d')).toMatchObject([{ kind: 'paragraph' }]);
    expect(blocksOf('Title\n---')).toMatchObject([{ kind: 'paragraph' }]);
  });

  it('ends at the first line without a pipe', () => {
    expect(blocksOf('| a |\n| - |\n| 1 |\nThat is all.').map((block) => block.kind)).toEqual([
      'table',
      'paragraph',
    ]);
  });
});

describe('blocksOf, links', () => {
  const GoNotes = 'https://go.dev/doc/go1.19';

  it('reads a link to a web page', () => {
    expect(blocksOf(`See the [Go 1.19 release notes](${GoNotes}).`)).toEqual([
      {
        kind: 'paragraph',
        spans: [
          { kind: 'text', text: 'See the ' },
          { href: GoNotes, kind: 'link', text: 'Go 1.19 release notes' },
          { kind: 'text', text: '.' },
        ],
      },
    ]);
  });

  it('reads a bare address, without the punctuation around it', () => {
    expect(blocksOf(`Read ${GoNotes}. Or the index (http://go.dev/doc/), then "https://pkg.go.dev"!`)).toEqual([
      {
        kind: 'paragraph',
        spans: [
          { kind: 'text', text: 'Read ' },
          { href: GoNotes, kind: 'link', text: GoNotes },
          { kind: 'text', text: '. Or the index (' },
          { href: 'http://go.dev/doc/', kind: 'link', text: 'http://go.dev/doc/' },
          { kind: 'text', text: '), then "' },
          { href: 'https://pkg.go.dev', kind: 'link', text: 'https://pkg.go.dev' },
          { kind: 'text', text: '"!' },
        ],
      },
    ]);
  });

  it('keeps a link to anything but a web page as text', () => {
    for (const text of ['[click](javascript:alert(1))', '[the file](src/a.ts)', '[mail](mailto:a@b.ch)']) {
      expect(blocksOf(text)).toEqual([{ kind: 'paragraph', spans: [{ kind: 'text', text }] }]);
    }
  });

  it('links an address written as code alone, and keeps code that holds one more as code', () => {
    expect(blocksOf(`\`${GoNotes}\``)).toEqual([
      { kind: 'paragraph', spans: [{ href: GoNotes, kind: 'link', look: 'code', text: GoNotes }] },
    ]);
    expect(blocksOf(`\`curl ${GoNotes}\``)).toEqual([
      { kind: 'paragraph', spans: [{ kind: 'code', text: `curl ${GoNotes}` }] },
    ]);
  });

  it('links an address or a link written in bold, as the agent wrote “running at **http://localhost:5174/**”', () => {
    expect(blocksOf('The docs run at **http://localhost:5174/jadis/**')).toEqual([
      {
        kind: 'paragraph',
        spans: [
          { kind: 'text', text: 'The docs run at ' },
          {
            href: 'http://localhost:5174/jadis/',
            kind: 'link',
            look: 'strong',
            text: 'http://localhost:5174/jadis/',
          },
        ],
      },
    ]);
    expect(blocksOf(`**[the notes](${GoNotes})**`)).toEqual([
      { kind: 'paragraph', spans: [{ href: GoNotes, kind: 'link', look: 'strong', text: 'the notes' }] },
    ]);
    expect(blocksOf('**Done** now')).toEqual([
      {
        kind: 'paragraph',
        spans: [
          { kind: 'strong', text: 'Done' },
          { kind: 'text', text: ' now' },
        ],
      },
    ]);
  });

  it('reads links in list items and table cells', () => {
    const [list, table] = blocksOf(`- [notes](${GoNotes})\n\n| Doc |\n| --- |\n| ${GoNotes} |`);
    expect(list?.kind === 'list' && list.items[0]).toEqual([{ href: GoNotes, kind: 'link', text: 'notes' }]);
    expect(table?.kind === 'table' && table.rows[0]).toEqual([[{ href: GoNotes, kind: 'link', text: GoNotes }]]);
  });
});
