/**
 * What a piece of code is, for its colour: the kinds jlib has a code colour for, then a diff's lines (added,
 * removed, and the line that says where a change is).
 */
export type TokenKind =
  | 'comment'
  | 'string'
  | 'number'
  | 'keyword'
  | 'function'
  | 'property'
  | 'tag'
  | 'attribute'
  | 'added'
  | 'removed'
  | 'meta';

/** A piece of a code block: its text, and what it is when that is known. */
export interface Token {
  text: string;
  kind?: TokenKind;
}

/** What a rule of a grammar finds: a kind of its own, or a word, which is a keyword, a call or plain. */
export type RuleKind = TokenKind | 'word';

/** One thing a grammar looks for, as the source of a regular expression. */
export interface Rule {
  kind: RuleKind;
  pattern: string;
}

/** How a language is read: its rules, the first that matches at a place winning, and its keywords. */
export interface Grammar {
  rules: readonly Rule[];
  keywords: ReadonlySet<string>;
  /** A word right before a `(` is a call. */
  calls: boolean;
}
