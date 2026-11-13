import type {
  Block,
  LineGroup,
  LinkLook,
  ListBlock,
  ListLine,
  Span,
  SpanMap,
  TableAlign,
} from '../types/markdown.types';

const FENCE = '```';
const LANGUAGE_LINE = /^([^\n]*)\n/;
const BLANK_LINES = /\n{2,}/;
const CODE_SPAN = /`[^`\n]+`/;
const STRONG_SPAN = /\*\*[^*\n]+\*\*/;
// To a web page only: a link with another scheme (javascript:, file:, a relative path) stays text.
const LINK_SPAN = /\[[^\]\n]+\]\(https?:\/\/[^\s)]+\)/;
// A bare address ends before the punctuation that ends a sentence or closes a bracket around it.
const BARE_URL = /https?:\/\/[^\s<>()[\]"']*[^\s<>()[\]"'.,;:!?]/;
const STRIKE_SPAN = /~~[^~\n]+~~/;
// Italics, by stars or by underscores: the marks touch the words they hold and nothing beside them, so a product
// (2 * 3 * 4), a glob (*.ts) and a name (my_file_name) stay as they are written.
const EM_STAR_SPAN = /(?<![*\w])\*(?![\s*])[^*\n]*[^\s*]\*(?![*\w])/;
const EM_UNDERSCORE_SPAN = /(?<![\w_])_(?![\s_])[^_\n]*[^\s_]_(?![\w_])/;
// One group around all of them: split keeps the parts they match. Where two start at the same place, the first
// listed wins: bold before italics.
const INLINE = new RegExp(
  `(${[CODE_SPAN, STRONG_SPAN, LINK_SPAN, BARE_URL, STRIKE_SPAN, EM_STAR_SPAN, EM_UNDERSCORE_SPAN]
    .map(({ source }) => source)
    .join('|')})`
);
const STRIKE_MARK = '~~';
const EM_MARK_LENGTH = 1;
const CODE_MARK = '`';
const STRONG_MARK = '**';
const LINK_OPEN = '[';
const LINK_MIDDLE = '](';
const LINK_CLOSE = ')';
const BULLET = /^\s*[-*+]\s+/;
const NUMBER = /^\s*\d+[.)]\s+/;
const QUOTE = /^\s*>\s?/;
// A rule across the message: a line of three dashes, stars or underscores or more, alone between blank lines
// (right under a line of text, dashes are that line's own).
const RULE = /^(?:-{3,}|\*{3,}|_{3,})$/;
const INDENT = /^[ \t]*/;
const TAB = '\t';
const TAB_WIDTH = 4;
const HEADING = /^(#{1,6})\s+/;
const PIPE = '|';
const ESCAPED_PIPE = '\\|';
const COLON = ':';
// A row's pieces: code spans and escaped pipes stay whole, so only the pipes between cells split it.
const ROW_PIECES = /(`[^`]*`|\\\||\|)/;
// A cell of the line under a table's header: dashes, a colon on the side the column lines up with.
const DELIMITER_CELL = /^:?-+:?$/;
// The header and the line under it: a table is at least these.
const TABLE_HEAD_LINES = 2;

const matchesWhole = (pattern: RegExp): RegExp => new RegExp(`^${pattern.source}$`);
const WHOLE_LINK = matchesWhole(LINK_SPAN);
const WHOLE_URL = matchesWhole(BARE_URL);
const WHOLE_STRIKE = matchesWhole(STRIKE_SPAN);
const WHOLE_EM = new RegExp(`^(?:${EM_STAR_SPAN.source}|${EM_UNDERSCORE_SPAN.source})$`);

/** `[text](href)`: the text has no `]`, so the first `](` ends it. */
const linkOf = (part: string): SpanMap['link'] => {
  const middle = part.indexOf(LINK_MIDDLE);
  return {
    href: part.slice(middle + LINK_MIDDLE.length, -LINK_CLOSE.length),
    kind: 'link',
    text: part.slice(LINK_OPEN.length, middle),
  };
};

/** Models often write an address in bold or as code: all of it an address, it is a link all the same. */
const addressLink = (inner: string, look: LinkLook): Span | undefined =>
  WHOLE_URL.test(inner) ? { href: inner, kind: 'link', look, text: inner } : undefined;

const spanOf = (part: string): Span => {
  if (part.startsWith(STRONG_MARK) && part.endsWith(STRONG_MARK) && part.length > STRONG_MARK.length * 2) {
    const strong = part.slice(STRONG_MARK.length, -STRONG_MARK.length);
    // A [text](address) in bold is a link too; in code, it is code someone may copy.
    const link: Span | undefined = WHOLE_LINK.test(strong) ? { ...linkOf(strong), look: 'strong' } : undefined;
    return link ?? addressLink(strong, 'strong') ?? { kind: 'strong', text: strong };
  }
  if (part.startsWith(CODE_MARK) && part.endsWith(CODE_MARK) && part.length > CODE_MARK.length * 2) {
    const code = part.slice(CODE_MARK.length, -CODE_MARK.length);
    return addressLink(code, 'code') ?? { kind: 'code', text: code };
  }
  if (WHOLE_LINK.test(part)) {
    return linkOf(part);
  }
  if (WHOLE_URL.test(part)) {
    return { href: part, kind: 'link', text: part };
  }
  if (WHOLE_STRIKE.test(part)) {
    return { kind: 'strike', text: part.slice(STRIKE_MARK.length, -STRIKE_MARK.length) };
  }
  if (WHOLE_EM.test(part)) {
    return { kind: 'em', text: part.slice(EM_MARK_LENGTH, -EM_MARK_LENGTH) };
  }
  return { kind: 'text', text: part };
};

/** A paragraph's text, with its `inline code`, **bold**, *italics*, ~~struck~~ words and links to web pages apart. */
export const spansOf = (text: string): Span[] =>
  text
    .split(INLINE)
    .filter((part) => part !== '')
    .map(spanOf);

const hasPipe = (line: string): boolean => line.includes(PIPE);

const endsWithBarePipe = (row: string): boolean => row.endsWith(PIPE) && !row.endsWith(ESCAPED_PIPE);

const pieceText = (piece: string): string => piece.replaceAll(ESCAPED_PIPE, PIPE);

/** A table row's cells: split on the pipes between them, the outer ones dropped; `\|` is a pipe in a cell. */
export function cellsOf(row: string): string[] {
  const trimmed = row.trim();
  const inner = trimmed.slice(trimmed.startsWith(PIPE) ? 1 : 0, endsWithBarePipe(trimmed) ? -1 : undefined);
  const cells = [''];
  for (const piece of inner.split(ROW_PIECES)) {
    if (piece === PIPE) {
      cells.push('');
      continue;
    }
    cells[cells.length - 1] += pieceText(piece);
  }
  return cells.map((cell) => cell.trim());
}

const isDelimiterRow = (line: string): boolean =>
  hasPipe(line) && cellsOf(line).every((cell) => DELIMITER_CELL.test(cell.replaceAll(' ', '')));

/**
 * How many lines of a table start here: a header, the line of dashes under it with as many cells, then the rows
 * that have a pipe. None when no table starts here.
 */
function tableLength(lines: readonly string[], start: number): number {
  const header = lines[start] ?? '';
  const delimiter = lines[start + 1] ?? '';
  if (!hasPipe(header) || !isDelimiterRow(delimiter) || cellsOf(header).length !== cellsOf(delimiter).length) {
    return 0;
  }
  const rows = lines.slice(start + TABLE_HEAD_LINES).findIndex((line) => !hasPipe(line));
  return rows === -1 ? lines.length - start : TABLE_HEAD_LINES + rows;
}

const alignOf = (cell: string): TableAlign => {
  const left = cell.startsWith(COLON);
  const right = cell.endsWith(COLON);
  if (left && right) {
    return 'center';
  }
  if (right) {
    return 'right';
  }
  return left ? 'left' : 'none';
};

/** A table's rows have the header's cells: missing ones are empty, extra ones dropped. */
function tableOf(lines: readonly string[]): Block {
  const [header = '', delimiter = '', ...rows] = lines;
  const columns = cellsOf(header).length;
  const toColumns = (cells: string[]): Span[][] =>
    Array.from({ length: columns }, (_, column) => spansOf(cells[column] ?? ''));
  return {
    align: cellsOf(delimiter).map((cell) => alignOf(cell.replaceAll(' ', ''))),
    header: toColumns(cellsOf(header)),
    kind: 'table',
    rows: rows.map((row) => toColumns(cellsOf(row))),
  };
}

const kindOf = (line: string): LineGroup['kind'] => {
  if (HEADING.test(line)) {
    return 'heading';
  }
  if (QUOTE.test(line)) {
    return 'quote';
  }
  return BULLET.test(line) || NUMBER.test(line) ? 'list' : 'text';
};

/** How far a line is indented, in spaces: a tab counts for four. */
const indentOf = (line: string): number =>
  [...(INDENT.exec(line)?.[0] ?? '')].reduce((width, mark) => width + (mark === TAB ? TAB_WIDTH : 1), 0);

const listLineOf = (line: string): ListLine => {
  const ordered = NUMBER.test(line);
  return { indent: indentOf(line), ordered, text: line.replace(ordered ? NUMBER : BULLET, '') };
};

/** A list with its nested lists, which it names only when an item has one. */
const listOf = (ordered: boolean, items: Span[][], nested: ListBlock[][]): ListBlock => ({
  items,
  kind: 'list',
  ordered,
  ...(nested.some((lists) => lists.length > 0) ? { nested } : {}),
});

/**
 * The lists a run of list lines makes: the lines indented as the first one are items, the deeper lines after an
 * item are the lists nested in it, and a list ends where bullets turn to numbers or back.
 */
function listsOf(lines: readonly ListLine[]): ListBlock[] {
  const base = lines[0]?.indent ?? 0;
  const lists: ListBlock[] = [];
  const open = { items: [] as Span[][], nested: [] as ListBlock[][], ordered: lines[0]?.ordered ?? false };
  const close = (): void => {
    if (open.items.length > 0) {
      lists.push(listOf(open.ordered, open.items, open.nested));
    }
  };
  let index = 0;
  while (index < lines.length) {
    const line = lines[index];
    if (!line) {
      break;
    }
    const after = lines.slice(index + 1);
    const deeper = after.findIndex((next) => next.indent <= base);
    const under = deeper < 0 ? after : after.slice(0, deeper);
    if (line.ordered !== open.ordered) {
      close();
      open.items = [];
      open.nested = [];
      open.ordered = line.ordered;
    }
    open.items.push(spansOf(line.text));
    open.nested.push(listsOf(under));
    index += 1 + under.length;
  }
  close();
  return lists;
}

/** Adds a line to the group it reads like, the last one; each heading stands alone. */
function addLine(groups: LineGroup[], line: string): void {
  const kind = kindOf(line);
  const last = groups.at(-1);
  if (last && last.kind === kind && kind !== 'heading') {
    last.lines.push(line);
    return;
  }
  groups.push({ kind, lines: [line] });
}

/** The lines of a paragraph, gathered while they read the same way; a table takes its lines whole. */
function groupsOf(lines: readonly string[]): LineGroup[] {
  const groups: LineGroup[] = [];
  let index = 0;
  while (index < lines.length) {
    const table = tableLength(lines, index);
    if (table > 0) {
      groups.push({ kind: 'table', lines: lines.slice(index, index + table) });
      index += table;
      continue;
    }
    addLine(groups, lines[index] ?? '');
    index += 1;
  }
  return groups;
}

const BLOCKS_OF_GROUPS: Readonly<Record<LineGroup['kind'], (lines: readonly string[]) => Block[]>> = {
  heading: (lines) => {
    const line = lines[0] ?? '';
    return [
      {
        kind: 'heading',
        level: HEADING.exec(line)?.[1]?.length ?? 1,
        spans: spansOf(line.replace(HEADING, '')),
      },
    ];
  },
  list: (lines) => listsOf(lines.map(listLineOf)),
  // What a quote holds is markdown of its own, under its marks.
  quote: (lines) => [
    { blocks: paragraphsOf(lines.map((line) => line.replace(QUOTE, '')).join('\n')), kind: 'quote' },
  ],
  table: (lines) => [tableOf(lines)],
  text: (lines) => [{ kind: 'paragraph', spans: spansOf(lines.join('\n').trimStart()) }],
};

// A paragraph keeps the indentation of its first line: a list may start indented, and its nesting is read from it.
const EDGES = /^\n+|\s+$/g;

function paragraphsOf(prose: string): Block[] {
  return prose
    .split(BLANK_LINES)
    .map((paragraph) => paragraph.replace(EDGES, ''))
    .filter((paragraph) => paragraph.trim() !== '')
    .flatMap((paragraph): Block[] =>
      RULE.test(paragraph.trim())
        ? [{ kind: 'rule' }]
        : groupsOf(paragraph.split('\n')).flatMap((group) => BLOCKS_OF_GROUPS[group.kind](group.lines))
    );
}

// The text after an opening fence starts with the language ("ts"), not with code.
const codeOf = (part: string): Block => {
  const language = LANGUAGE_LINE.exec(part)?.[1]?.trim().toLowerCase() ?? '';
  return {
    kind: 'code',
    text: part.replace(LANGUAGE_LINE, '').replace(/\n$/, ''),
    ...(language === '' ? {} : { language }),
  };
};

/**
 * The markdown models write: code fences, headings, lists and the lists nested in them, tables, quotes, rules,
 * paragraphs, inline code, bold, italics, struck words and links. A fence still open while the answer streams
 * shows as code already.
 */
export const blocksOf = (text: string): Block[] =>
  text.split(FENCE).flatMap((part, index) => (index % 2 === 1 ? [codeOf(part)] : paragraphsOf(part)));

/**
 * How many blocks at the start of a message are the same as the ones drawn before it grew: an answer streams in
 * at its end, so all but its last block or two stay as they are, and only what follows them is drawn again.
 * `drawn` and `next` are each block's text form, as `blockKey` gives it.
 */
export function sameBlocks(drawn: readonly string[], next: readonly string[]): number {
  const shared = Math.min(drawn.length, next.length);
  const changed = next.slice(0, shared).findIndex((key, index) => key !== drawn[index]);
  return changed < 0 ? shared : changed;
}

/** A block as one text, to tell whether it is the same as the one drawn before. */
export const blockKey = (block: Block): string => JSON.stringify(block);
