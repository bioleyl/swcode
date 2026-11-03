import { css } from '@jadis/core';

import { oneOf, ScElement } from '../base/sc-element';
import { blocksOf } from '../lib/markdown';
import { BASE_STYLES } from '../styles/shared';
import styles from './sc-message.css?inline';

import type { Block, BlockMap, LinkLook, Span, SpanMap } from '../types/markdown.types';

export const MESSAGE_AUTHORS = ['assistant', 'user'] as const;
export type MessageAuthor = (typeof MESSAGE_AUTHORS)[number];

// A link written all as code or all in bold keeps that look inside the link.
const LINK_LOOKS: Readonly<Record<LinkLook, (text: string) => Node>> = {
  code: (text) => <code>{text}</code>,
  strong: (text) => <strong>{text}</strong>,
};

const SPAN_NODES: { [Kind in keyof SpanMap]: (span: SpanMap[Kind]) => Node | string } = {
  code: (span) => <code>{span.text}</code>,
  // In a tab of its own: the session stays open behind it.
  link: (span) => (
    <a href={span.href} target="_blank" rel="noopener">
      {span.look ? LINK_LOOKS[span.look](span.text) : span.text}
    </a>
  ),
  strong: (span) => <strong>{span.text}</strong>,
  text: (span) => span.text,
};

const spanNodeOf = <Kind extends keyof SpanMap>(kind: Kind, span: SpanMap[Kind]): Node | string =>
  SPAN_NODES[kind](span);

const spanNode = (span: Span): Node | string => spanNodeOf(span.kind, span);

// Headings stay modest inside a message: two sizes only.
const MAJOR_HEADING_LEVEL = 2;

const BLOCK_NODES: { [Kind in keyof BlockMap]: (block: BlockMap[Kind]) => Node } = {
  code: (block) => (
    <pre>
      <code>{block.text}</code>
    </pre>
  ),
  heading: (block) =>
    block.level <= MAJOR_HEADING_LEVEL ? (
      <h3>{block.spans.map(spanNode)}</h3>
    ) : (
      <h4>{block.spans.map(spanNode)}</h4>
    ),
  list: (block) => {
    const items = block.items.map((spans) => <li>{spans.map(spanNode)}</li>);
    return block.ordered ? <ol>{items}</ol> : <ul>{items}</ul>;
  },
  paragraph: (block) => <p>{block.spans.map(spanNode)}</p>,
  // In a box of its own, which scrolls sideways when the table is wider than the message.
  table: (block) => (
    <div class="table">
      <table>
        <thead>
          <tr>
            {block.header.map((cell, column) => (
              <th data-align={block.align[column] ?? 'none'}>{cell.map(spanNode)}</th>
            ))}
          </tr>
        </thead>
        <tbody>
          {block.rows.map((row) => (
            <tr>
              {row.map((cell, column) => (
                <td data-align={block.align[column] ?? 'none'}>{cell.map(spanNode)}</td>
              ))}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  ),
};

const nodeOf = <Kind extends keyof BlockMap>(kind: Kind, block: BlockMap[Kind]): Node => BLOCK_NODES[kind](block);

const blockNode = (block: Block): Node => nodeOf(block.kind, block);

/** A message of the conversation: the person's, in a quiet box, or the agent's, as plain text. */
export class ScMessage extends ScElement {
  static readonly selector = 'sc-message';

  readonly author = this.useChange<MessageAuthor>('assistant', (value) => this.reflect('author', value), {
    attribute: 'author',
    parse: oneOf(MESSAGE_AUTHORS, 'assistant'),
  });
  readonly text = this.useChange('', (value) =>
    this._refs.body.replaceChildren(...blocksOf(value).map(blockNode))
  );
  /** The addresses of the pictures pasted with it: a click opens one whole, in a tab. */
  readonly pictures = this.useChange<string[]>([], (urls) => {
    this._refs.pictures.hidden = urls.length === 0;
    this._refs.pictures.replaceChildren(
      ...urls.map((url, index) => (
        <a href={url} target="_blank" rel="noopener">
          <img src={url} alt={`Pasted, ${index + 1} of ${urls.length}`} loading="lazy" />
        </a>
      ))
    );
  });

  private readonly _refs = this.useRefs((ref) => ({ body: ref('.body'), pictures: ref('.pictures') }));

  /** Adds a piece of an answer as it streams in. */
  append(text: string): void {
    this.text.set((sofar) => `${sofar}${text}`);
  }

  templateHtml(): Node {
    return (
      <>
        <div class="body" />
        <div class="pictures" hidden />
      </>
    );
  }

  templateCss(): string {
    return css`${BASE_STYLES}${styles}`;
  }
}

ScMessage.register();
