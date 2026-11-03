/** How a link was written when it was all of a code or a bold part: it keeps that look. */
export type LinkLook = 'code' | 'strong';

/** A piece of a paragraph's text: plain, inline code, bold, or a link to a web page. */
export type Span =
  | { kind: 'text' | 'code' | 'strong'; text: string }
  | { kind: 'link'; text: string; href: string; look?: LinkLook };

export type SpanMap = { [Each in Span as Each['kind']]: Each };

/** How a table's column lines up, from the colons of the line under its header. */
export type TableAlign = 'none' | 'left' | 'center' | 'right';

/** A piece of a message: a code block, a heading, a list, a table, or a paragraph with inline code, bold and links. */
export type Block =
  | { kind: 'code'; text: string }
  | { kind: 'heading'; level: number; spans: Span[] }
  | { kind: 'list'; ordered: boolean; items: Span[][] }
  | { kind: 'table'; align: TableAlign[]; header: Span[][]; rows: Span[][][] }
  | { kind: 'paragraph'; spans: Span[] };

export type BlockMap = { [Each in Block as Each['kind']]: Each };

/** Consecutive lines of a paragraph that read the same way: plain text, list items, a heading, or a table. */
export interface LineGroup {
  kind: 'text' | 'bullet' | 'number' | 'heading' | 'table';
  lines: string[];
}
