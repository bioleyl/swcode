import { describe, expect, it } from 'vitest';

import { tokensOf } from '../src/lib/highlight';
import { blockKey, blocksOf, sameBlocks, spansOf } from '../src/lib/markdown';

import type { Token } from '../src/types/highlight.types';

/** The pieces that have a kind, as "kind:text". */
const marked = (tokens: readonly Token[]): string[] =>
  tokens.filter((token) => token.kind !== undefined).map((token) => `${token.kind}:${token.text}`);

const whole = (tokens: readonly Token[]): string => tokens.map((token) => token.text).join('');

describe('tokensOf', () => {
  it('finds a language’s comments, strings, numbers, own words and calls', () => {
    const code = 'const total = add(40, 2); // the answer\nreturn `it is done`;';
    const tokens = tokensOf(code, 'ts');
    expect(marked(tokens)).toEqual([
      'keyword:const',
      'function:add',
      'number:40',
      'number:2',
      'comment:// the answer',
      'keyword:return',
      'string:`it is done`',
    ]);
    expect(whole(tokens)).toBe(code);
  });

  it('keeps every character, whatever the language, and joins plain text into few pieces', () => {
    const samples: Readonly<Record<string, string>> = {
      bash: 'export PATH="$HOME/bin:$PATH" # mine\nnpm run build --watch',
      css: '.card { color: var(--jl-text); margin: 4px 1.5rem; } /* done */',
      go: 'func main() {\n\tfmt.Println("hi", 0x2A)\n}',
      html: '<a class="link" href="/x">go</a><!-- end -->',
      json: '{ "name": "swcode", "port": 4500, "open": true }',
      python: 'def greet(name):\n    """Says hello."""\n    return f"Hi {name}"  # done',
      rust: 'fn main<\'a>() { let total: u32 = 42; println!("{}", total); }',
      yaml: 'name: swcode\nport: 4500 # the default\nopen: true',
    };
    for (const [language, code] of Object.entries(samples)) {
      const tokens = tokensOf(code, language);
      expect(whole(tokens), language).toBe(code);
      expect(marked(tokens).length, language).toBeGreaterThan(0);
      // No two plain pieces follow each other.
      expect(
        tokens.some(
          (token, index) =>
            token.kind === undefined
            && tokens[index + 1]?.text !== undefined
            && tokens[index + 1]?.kind === undefined
        ),
        language
      ).toBe(false);
    }
  });

  it('reads each language by its own rules', () => {
    expect(marked(tokensOf('{ "name": "swcode", "port": 4500, "open": true }', 'json'))).toEqual([
      'property:"name"',
      'string:"swcode"',
      'property:"port"',
      'number:4500',
      'property:"open"',
      'keyword:true',
    ]);
    expect(marked(tokensOf('echo "$# args" # count\nls -la $HOME', 'sh'))).toEqual([
      'keyword:echo',
      'string:"$# args"',
      'comment:# count',
      'attribute:-la',
      'property:$HOME',
    ]);
    expect(marked(tokensOf('<a href="/x">go</a>', 'html'))).toEqual([
      'tag:<a',
      'attribute:href',
      'string:"/x"',
      'tag:>',
      'tag:</a',
      'tag:>',
    ]);
    expect(marked(tokensOf('port: 4500 # the default', 'yaml'))).toEqual([
      'property:port',
      'number:4500',
      'comment:# the default',
    ]);
    expect(marked(tokensOf('def greet(name):\n    return None', 'python'))).toEqual([
      'keyword:def',
      'function:greet',
      'keyword:return',
      'keyword:None',
    ]);
  });

  it('colours a diff by its lines', () => {
    const diff = 'diff --git a/a.ts b/a.ts\n@@ -1 +1 @@\n-const a = 1;\n+const a = 2;\n same';
    expect(tokensOf(diff, 'diff')).toEqual([
      { kind: 'meta', text: 'diff --git a/a.ts b/a.ts\n' },
      { kind: 'meta', text: '@@ -1 +1 @@\n' },
      { kind: 'removed', text: '-const a = 1;\n' },
      { kind: 'added', text: '+const a = 2;\n' },
      { text: ' same' },
    ]);
  });

  it('leaves a language it does not read, no language, and a very long block as they are', () => {
    expect(tokensOf('const a = 1;', 'brainfuck')).toEqual([{ text: 'const a = 1;' }]);
    expect(tokensOf('const a = 1;', undefined)).toEqual([{ text: 'const a = 1;' }]);
    const long = 'const a = 1;\n'.repeat(4000);
    expect(tokensOf(long, 'ts')).toEqual([{ text: long }]);
    expect(tokensOf('const a = 1;', ' TS ').some((token) => token.kind === 'keyword')).toBe(true);
  });

  it('colours a string or a comment still open while the answer streams in', () => {
    expect(marked(tokensOf('const a = "unfinished', 'ts'))).toEqual(['keyword:const', 'string:"unfinished']);
    expect(marked(tokensOf('/* still writing', 'ts'))).toEqual(['comment:/* still writing']);
  });
});

describe('spansOf, beyond bold and code', () => {
  it('reads italics by stars and by underscores, and struck words', () => {
    expect(spansOf('It is *really* _very_ ~~bad~~ good.')).toEqual([
      { kind: 'text', text: 'It is ' },
      { kind: 'em', text: 'really' },
      { kind: 'text', text: ' ' },
      { kind: 'em', text: 'very' },
      { kind: 'text', text: ' ' },
      { kind: 'strike', text: 'bad' },
      { kind: 'text', text: ' good.' },
    ]);
    expect(spansOf('**bold** then *a few words*')).toEqual([
      { kind: 'strong', text: 'bold' },
      { kind: 'text', text: ' then ' },
      { kind: 'em', text: 'a few words' },
    ]);
  });

  it('leaves a product, a glob and a name with underscores as they are written', () => {
    for (const text of ['2 * 3 * 4', 'the *.ts and *.js files', 'my_file_name.ts', '__init__', 'a_b and c_d']) {
      expect(spansOf(text), text).toEqual([{ kind: 'text', text }]);
    }
    expect(spansOf('`a_b_c` https://example.com/a_b_c')).toEqual([
      { kind: 'code', text: 'a_b_c' },
      { kind: 'text', text: ' ' },
      { href: 'https://example.com/a_b_c', kind: 'link', text: 'https://example.com/a_b_c' },
    ]);
  });
});

describe('blocksOf, the rest of markdown', () => {
  it('names a code block’s language, when its fence names one', () => {
    expect(blocksOf('```TS\nconst a = 1;\n```')).toEqual([{ kind: 'code', language: 'ts', text: 'const a = 1;' }]);
    expect(blocksOf('```\nplain\n```')).toEqual([{ kind: 'code', text: 'plain' }]);
  });

  it('nests the lists indented under an item', () => {
    expect(blocksOf('- one\n  - inner a\n  - inner b\n- two\n  1. first\n  2. second')).toEqual([
      {
        items: [[{ kind: 'text', text: 'one' }], [{ kind: 'text', text: 'two' }]],
        kind: 'list',
        nested: [
          [
            {
              items: [[{ kind: 'text', text: 'inner a' }], [{ kind: 'text', text: 'inner b' }]],
              kind: 'list',
              ordered: false,
            },
          ],
          [
            {
              items: [[{ kind: 'text', text: 'first' }], [{ kind: 'text', text: 'second' }]],
              kind: 'list',
              ordered: true,
            },
          ],
        ],
        ordered: false,
      },
    ]);
  });

  it('nests three deep, and keeps a list that starts indented', () => {
    const [list] = blocksOf('  1. a\n     - b\n       - c\n  2. d');
    expect(list).toMatchObject({
      items: [[{ text: 'a' }], [{ text: 'd' }]],
      nested: [[{ items: [[{ text: 'b' }]], nested: [[{ items: [[{ text: 'c' }]] }]] }], []],
      ordered: true,
    });
  });

  it('ends a list where bullets turn to numbers at the same depth', () => {
    expect(blocksOf('- a\n- b\n1. c').map((block) => (block.kind === 'list' ? block.ordered : undefined))).toEqual(
      [false, true]
    );
  });

  it('reads a quote as markdown of its own', () => {
    expect(blocksOf('> **Note**\n> - one\n> - two\n\nAfter.')).toEqual([
      {
        blocks: [
          { kind: 'paragraph', spans: [{ kind: 'strong', text: 'Note' }] },
          {
            items: [[{ kind: 'text', text: 'one' }], [{ kind: 'text', text: 'two' }]],
            kind: 'list',
            ordered: false,
          },
        ],
        kind: 'quote',
      },
      { kind: 'paragraph', spans: [{ kind: 'text', text: 'After.' }] },
    ]);
  });

  it('draws a rule for a line of dashes alone, not for one right under a line of text', () => {
    expect(blocksOf('Above.\n\n---\n\nBelow.').map((block) => block.kind)).toEqual([
      'paragraph',
      'rule',
      'paragraph',
    ]);
    expect(blocksOf('***').map((block) => block.kind)).toEqual(['rule']);
    expect(blocksOf('Title\n---').map((block) => block.kind)).toEqual(['paragraph']);
  });

  it('keeps the blocks before the one that grows as an answer streams in', () => {
    const before = blocksOf('> a quote\n\n- one\n  - inner\n\n```ts\nconst a').map(blockKey);
    const after = blocksOf('> a quote\n\n- one\n  - inner\n\n```ts\nconst a = 1;').map(blockKey);
    expect(sameBlocks(before, after)).toBe(2);
  });
});
