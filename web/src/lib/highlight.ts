import type { Grammar, Rule, Token, TokenKind } from '../types/highlight.types';

// A block longer than this stays plain: it is drawn again at each frame while it streams in.
const MAX_HIGHLIGHTED = 40_000;

const LINE_COMMENT = '\\/\\/[^\\n]*';
const BLOCK_COMMENT = '\\/\\*[\\s\\S]*?(?:\\*\\/|$)';
const HASH_COMMENT = '#[^\\n]*';
const DOUBLE_QUOTED = '"(?:\\\\.|[^"\\\\\\n])*"?';
const SINGLE_QUOTED = "'(?:\\\\.|[^'\\\\\\n])*'?";
const BACK_QUOTED = '`(?:\\\\.|[^`\\\\])*`?';
const NUMBER = '\\b(?:0[xX][\\da-fA-F_]+|\\d[\\d_]*(?:\\.\\d+)?(?:[eE][+-]?\\d+)?)\\b';
const WORD = '[A-Za-z_$][\\w$]*';

const rule = (kind: Rule['kind'], pattern: string): Rule => ({ kind, pattern });
const words = (list: string): ReadonlySet<string> => new Set(list.split(' '));

const C_LIKE_RULES: readonly Rule[] = [
  rule('comment', `${LINE_COMMENT}|${BLOCK_COMMENT}`),
  rule('string', `${DOUBLE_QUOTED}|${SINGLE_QUOTED}|${BACK_QUOTED}`),
  rule('number', NUMBER),
  rule('word', WORD),
];

const SCRIPT: Grammar = {
  calls: true,
  keywords: words(
    'abstract as async await break case catch class const continue debugger declare default delete do else enum export extends false finally for from function get if implements import in instanceof interface is keyof let namespace new null of private protected public readonly return satisfies set static super switch this throw true try type typeof undefined var void while yield'
  ),
  rules: C_LIKE_RULES,
};

const GO: Grammar = {
  calls: true,
  keywords: words(
    'break case chan const continue default defer else fallthrough false for func go goto if import interface map nil package range return select struct switch true type var any bool byte error float32 float64 int int32 int64 rune string uint uint8 uint32 uint64'
  ),
  rules: C_LIKE_RULES,
};

const RUST: Grammar = {
  calls: true,
  keywords: words(
    'as async await break const continue crate dyn else enum extern false fn for if impl in let loop match mod move mut pub ref return self Self static struct super trait true type unsafe use where while'
  ),
  rules: [
    rule('comment', `${LINE_COMMENT}|${BLOCK_COMMENT}`),
    // A single quote also starts a lifetime ('a): only a double quote is a string here.
    rule('string', DOUBLE_QUOTED),
    rule('number', NUMBER),
    rule('word', WORD),
  ],
};

const PYTHON: Grammar = {
  calls: true,
  keywords: words(
    'and as assert async await break class continue def del elif else except False finally for from global if import in is lambda None nonlocal not or pass raise return self True try while with yield'
  ),
  rules: [
    rule('comment', HASH_COMMENT),
    rule('string', `"""[\\s\\S]*?(?:"""|$)|'''[\\s\\S]*?(?:'''|$)|${DOUBLE_QUOTED}|${SINGLE_QUOTED}`),
    rule('number', NUMBER),
    rule('word', WORD),
  ],
};

const SHELL: Grammar = {
  calls: false,
  keywords: words(
    'case do done elif else esac export fi for function if in local return set then unset until while cd echo exit source sudo'
  ),
  rules: [
    // A # starts a comment at the start of a word only: $# and ${#name} are not one.
    rule('comment', `(?<![\\w$\{])${HASH_COMMENT}`),
    rule('string', `${DOUBLE_QUOTED}|${SINGLE_QUOTED}`),
    rule('property', '\\$\\{[^}\\n]*\\}|\\$[\\w@#?*!]+'),
    rule('attribute', '(?<=\\s)--?[A-Za-z][\\w-]*'),
    rule('number', NUMBER),
    rule('word', '[A-Za-z_][\\w.-]*'),
  ],
};

const JSON_GRAMMAR: Grammar = {
  calls: false,
  keywords: words('true false null'),
  rules: [
    rule('comment', `${LINE_COMMENT}|${BLOCK_COMMENT}`),
    rule('property', `${DOUBLE_QUOTED}(?=\\s*:)`),
    rule('string', DOUBLE_QUOTED),
    rule('number', `-?${NUMBER}`),
    rule('word', WORD),
  ],
};

const CSS: Grammar = {
  calls: true,
  keywords: words('important inherit initial unset none auto'),
  rules: [
    rule('comment', BLOCK_COMMENT),
    rule('string', `${DOUBLE_QUOTED}|${SINGLE_QUOTED}`),
    rule('keyword', '@[\\w-]+'),
    rule('property', '--[\\w-]+|[a-z-]+(?=\\s*:)'),
    rule('number', '#[\\da-fA-F]{3,8}\\b|-?\\d*\\.?\\d+(?:%|[a-z]+)?'),
    rule('word', '[A-Za-z_][\\w-]*'),
  ],
};

const HTML: Grammar = {
  calls: false,
  keywords: words(''),
  rules: [
    rule('comment', '<!--[\\s\\S]*?(?:-->|$)'),
    rule('tag', '<\\/?[A-Za-z][\\w:-]*|\\/?>'),
    rule('string', `${DOUBLE_QUOTED}|${SINGLE_QUOTED}`),
    rule('attribute', '[A-Za-z_:@][\\w:.-]*(?==)'),
  ],
};

const YAML: Grammar = {
  calls: false,
  keywords: words('true false null yes no on off'),
  rules: [
    rule('comment', `(?<![\\w"'])${HASH_COMMENT}`),
    rule('string', `${DOUBLE_QUOTED}|${SINGLE_QUOTED}`),
    rule('property', '(?<=^[ \\t-]*)[\\w./-]+(?=\\s*:(?:\\s|$))'),
    rule('number', NUMBER),
    rule('word', WORD),
  ],
};

/** The names a fence gives a language, each with its grammar: another name, and the block stays plain. */
const GRAMMARS: Readonly<Record<string, Grammar>> = {
  bash: SHELL,
  cjs: SCRIPT,
  console: SHELL,
  css: CSS,
  go: GO,
  golang: GO,
  htm: HTML,
  html: HTML,
  javascript: SCRIPT,
  js: SCRIPT,
  json: JSON_GRAMMAR,
  json5: JSON_GRAMMAR,
  jsonc: JSON_GRAMMAR,
  jsx: SCRIPT,
  mjs: SCRIPT,
  py: PYTHON,
  python: PYTHON,
  rs: RUST,
  rust: RUST,
  scss: CSS,
  sh: SHELL,
  shell: SHELL,
  svg: HTML,
  ts: SCRIPT,
  tsx: SCRIPT,
  typescript: SCRIPT,
  xml: HTML,
  yaml: YAML,
  yml: YAML,
  zsh: SHELL,
};

const DIFF_NAMES: ReadonlySet<string> = new Set(['diff', 'patch']);

// What a diff's line starts with says what it is; the file lines (+++ and ---) say where, like the @@ ones.
const DIFF_LINES: readonly { starts: string; kind: TokenKind }[] = [
  { kind: 'meta', starts: '+++' },
  { kind: 'meta', starts: '---' },
  { kind: 'meta', starts: '@@' },
  { kind: 'meta', starts: 'diff ' },
  { kind: 'meta', starts: 'index ' },
  { kind: 'added', starts: '+' },
  { kind: 'removed', starts: '-' },
];

const CALL_AHEAD = /^\s*\(/;

const plain = (text: string): Token => ({ text });

/** A diff, a line each: added, removed, where the change is, or as it is. */
function diffTokens(code: string): Token[] {
  return code.split('\n').map((line, index, lines) => {
    const text = index < lines.length - 1 ? `${line}\n` : line;
    const kind = DIFF_LINES.find(({ starts }) => line.startsWith(starts))?.kind;
    return kind ? { kind, text } : plain(text);
  });
}

const finders = new Map<Grammar, RegExp>();

/** One expression for a grammar's rules, a group each: the group that matched says which rule did. */
function finderOf(grammar: Grammar): RegExp {
  const known = finders.get(grammar);
  if (known) {
    return known;
  }
  const finder = new RegExp(grammar.rules.map(({ pattern }) => `(${pattern})`).join('|'), 'gm');
  finders.set(grammar, finder);
  return finder;
}

/** What a word is: one of the language's own, a call, or a name like any other. */
function wordKind(grammar: Grammar, word: string, rest: string): TokenKind | undefined {
  if (grammar.keywords.has(word)) {
    return 'keyword';
  }
  return grammar.calls && CALL_AHEAD.test(rest) ? 'function' : undefined;
}

/** Adds a piece to the tokens: plain text joins the plain text before it, so a block is few nodes. */
function push(tokens: Token[], text: string, kind: TokenKind | undefined): void {
  if (text === '') {
    return;
  }
  const last = tokens.at(-1);
  if (kind === undefined && last && last.kind === undefined) {
    last.text += text;
    return;
  }
  tokens.push(kind ? { kind, text } : plain(text));
}

function grammarTokens(code: string, grammar: Grammar): Token[] {
  const tokens: Token[] = [];
  let at = 0;
  for (const match of code.matchAll(finderOf(grammar))) {
    const text = match[0];
    if (text === '') {
      continue;
    }
    const start = match.index;
    push(tokens, code.slice(at, start), undefined);
    const found = grammar.rules[match.slice(1).findIndex((group) => group !== undefined)]?.kind;
    const kind = found === 'word' ? wordKind(grammar, text, code.slice(start + text.length)) : found;
    push(tokens, text, kind);
    at = start + text.length;
  }
  push(tokens, code.slice(at), undefined);
  return tokens;
}

/**
 * A code block's text in pieces, each with what it is in the language its fence names: comments, strings,
 * numbers, the language's own words, calls. A language swcode does not read, no language at all, or a very long
 * block: one plain piece.
 */
export function tokensOf(code: string, language: string | undefined): Token[] {
  const name = language?.trim().toLowerCase() ?? '';
  if (code === '' || code.length > MAX_HIGHLIGHTED) {
    return [plain(code)];
  }
  if (DIFF_NAMES.has(name)) {
    return diffTokens(code);
  }
  const grammar = GRAMMARS[name];
  return grammar ? grammarTokens(code, grammar) : [plain(code)];
}
