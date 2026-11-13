/** How a link was written when it was all of a code or a bold part: it keeps that look. */
export type LinkLook = 'code' | 'strong';

/** A piece of a paragraph's text: plain, inline code, bold, italic, struck through, or a link to a web page. */
export type Span =
  | { kind: 'text' | 'code' | 'strong' | 'em' | 'strike'; text: string }
  | { kind: 'link'; text: string; href: string; look?: LinkLook };

export type SpanMap = { [Each in Span as Each['kind']]: Each };

/** How a table's column lines up, from the colons of the line under its header. */
export type TableAlign = 'none' | 'left' | 'center' | 'right';

/**
 * A list: its items, and under each the lists nested in it. `nested` is there only when an item has one, and then
 * holds an entry for every item, empty for those without.
 */
export interface ListBlock {
  kind: 'list';
  ordered: boolean;
  items: Span[][];
  nested?: ListBlock[][];
}

/**
 * A piece of a message: a code block (with the language its fence names, when it names one), a heading, a list, a
 * table, a quote holding blocks of its own, a rule across the message, or a paragraph with its spans.
 */
export type Block =
  | { kind: 'code'; text: string; language?: string }
  | { kind: 'heading'; level: number; spans: Span[] }
  | ListBlock
  | { kind: 'table'; align: TableAlign[]; header: Span[][]; rows: Span[][][] }
  | { kind: 'quote'; blocks: Block[] }
  | { kind: 'rule' }
  | { kind: 'paragraph'; spans: Span[] };

export type BlockMap = { [Each in Block as Each['kind']]: Each };

/** Consecutive lines of a paragraph that read the same way: plain text, list items, a heading, a quote or a table. */
export interface LineGroup {
  kind: 'text' | 'list' | 'heading' | 'table' | 'quote';
  lines: string[];
}

/** A line of a list: how far it is indented, whether it is numbered, and what it says. */
export interface ListLine {
  indent: number;
  ordered: boolean;
  text: string;
}
