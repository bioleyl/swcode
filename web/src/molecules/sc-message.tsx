import { createElement, css } from '@jadis/core';
import { JlButton } from '@syware/jlib';

import { oneOf, ScElement } from '../base/sc-element';
import { copyText } from '../lib/clipboard';
import { tokensOf } from '../lib/highlight';
import { blockKey, blocksOf, sameBlocks } from '../lib/markdown';
import { BASE_STYLES } from '../styles/shared';
import styles from './sc-message.css?inline';

import type { Token } from '../types/highlight.types';
import type { Block, BlockMap, LinkLook, ListBlock, Span, SpanMap } from '../types/markdown.types';

export const MESSAGE_AUTHORS = ['assistant', 'user'] as const;
export type MessageAuthor = (typeof MESSAGE_AUTHORS)[number];

const PIN_HINT = 'Pin this message';
const UNPIN_HINT = 'Remove this pin';
const COPY_HINT = 'Copy';
const COPIED_HINT = 'Copied';
const NOT_COPIED_HINT = 'The browser did not let swcode copy';
// How long a code block's button says its text was copied.
const COPIED_MS = 1500;

// A link written all as code or all in bold keeps that look inside the link.
const LINK_LOOKS: Readonly<Record<LinkLook, (text: string) => Node>> = {
  code: (text) => <code>{text}</code>,
  strong: (text) => <strong>{text}</strong>,
};

const SPAN_NODES: { [Kind in keyof SpanMap]: (span: SpanMap[Kind]) => Node | string } = {
  code: (span) => <code>{span.text}</code>,
  em: (span) => <em>{span.text}</em>,
  // In a tab of its own: the session stays open behind it.
  link: (span) => (
    <a href={span.href} target="_blank" rel="noopener">
      {span.look ? LINK_LOOKS[span.look](span.text) : span.text}
    </a>
  ),
  strike: (span) => <del>{span.text}</del>,
  strong: (span) => <strong>{span.text}</strong>,
  text: (span) => span.text,
};

const spanNodeOf = <Kind extends keyof SpanMap>(kind: Kind, span: SpanMap[Kind]): Node | string =>
  SPAN_NODES[kind](span);

const spanNode = (span: Span): Node | string => spanNodeOf(span.kind, span);

// Headings stay modest inside a message: two sizes only.
const MAJOR_HEADING_LEVEL = 2;

/** A piece of code: in its colour when the language says what it is, as it is otherwise. */
const tokenNode = (token: Token): Node | string =>
  token.kind ? <span class={`tok-${token.kind}`}>{token.text}</span> : token.text;

/** Copies a code block, and says so on its button for a moment. */
async function copyCode(button: JlButton, text: string): Promise<void> {
  const copied = await copyText(text);
  button.icon.set(copied ? 'check' : 'circle-alert');
  button.hint.set(copied ? COPIED_HINT : NOT_COPIED_HINT);
  setTimeout(() => {
    button.icon.set('copy');
    button.hint.set(COPY_HINT);
  }, COPIED_MS);
}

/** A code block: its text in the colours of its language, and a button that copies it. One node, as every block. */
function codeNode(block: BlockMap['code']): Node {
  // A button keeps its clicks to itself, so each knows its block.
  const copy = createElement(JlButton, {
    attrs: { class: 'copy' },
    props: { hint: COPY_HINT, icon: 'copy', size: 'sm', variant: 'ghost' },
  });
  copy.events.register('press', () => void copyCode(copy, block.text));
  return (
    <div class="code">
      {copy}
      <pre>
        <code>{tokensOf(block.text, block.language).map(tokenNode)}</code>
      </pre>
    </div>
  );
}

/** A list, each item with the lists nested in it. */
function listNode(block: ListBlock): Node {
  const items = block.items.map((spans, index) => (
    <li>
      {spans.map(spanNode)}
      {(block.nested?.[index] ?? []).map(listNode)}
    </li>
  ));
  return block.ordered ? <ol>{items}</ol> : <ul>{items}</ul>;
}

const BLOCK_NODES: { [Kind in keyof BlockMap]: (block: BlockMap[Kind]) => Node } = {
  code: codeNode,
  heading: (block) =>
    block.level <= MAJOR_HEADING_LEVEL ? (
      <h3>{block.spans.map(spanNode)}</h3>
    ) : (
      <h4>{block.spans.map(spanNode)}</h4>
    ),
  list: listNode,
  paragraph: (block) => <p>{block.spans.map(spanNode)}</p>,
  quote: (block) => <blockquote>{block.blocks.map(blockNode)}</blockquote>,
  rule: () => <hr />,
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

function blockNode(block: Block): Node {
  return nodeOf(block.kind, block);
}

/** A message of the conversation: the person's, in a quiet box, or the agent's, as plain text. */
export class ScMessage extends ScElement {
  static readonly selector = 'sc-message';

  readonly events = this.useEvents<{ pin: undefined }>();

  readonly author = this.useChange<MessageAuthor>('assistant', (value) => this.reflect('author', value), {
    attribute: 'author',
    parse: oneOf(MESSAGE_AUTHORS, 'assistant'),
  });
  /** Whether the message can be pinned: the conversation's can, what waits in the queue cannot. */
  readonly pinnable = this.useChange(
    false,
    (pinnable) => {
      this._refs.pin.hidden = !pinnable;
    },
    { attribute: 'pinnable' }
  );
  /** Whether a pin points here: the button is shown, and takes it off. */
  readonly pinned = this.useChange(
    false,
    (pinned) => {
      this.reflect('pinned', pinned);
      this._refs.pin.icon.set(pinned ? 'pin-off' : 'pin');
      this._refs.pin.hint.set(pinned ? UNPIN_HINT : PIN_HINT);
    },
    { attribute: 'pinned' }
  );
  readonly text = this.useChange('', (value) => this.drawText(value));
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

  // The blocks drawn, each as its text form and in the order of the body's children: a longer text draws again
  // only the blocks that changed, which is the last one or two of an answer as it streams in.
  private _drawn: string[] = [];

  private readonly _refs = this.useRefs((ref) => ({
    body: ref('.body'),
    pictures: ref('.pictures'),
    pin: ref(JlButton),
  }));

  /** Draws the text's blocks: those the body already shows stay, the ones after them are made again. */
  private drawText(value: string): void {
    const blocks = blocksOf(value);
    const keys = blocks.map(blockKey);
    const kept = sameBlocks(this._drawn, keys);
    const body = this._refs.body;
    while (body.childNodes.length > kept) {
      body.lastChild?.remove();
    }
    body.append(...blocks.slice(kept).map(blockNode));
    this._drawn = keys;
  }

  /** Adds a piece of an answer as it streams in. */
  append(text: string): void {
    this.text.set((sofar) => `${sofar}${text}`);
  }

  onConnect(): void {
    this._refs.pin.events.register('press', () => this.events.emit('pin'));
  }

  templateHtml(): Node {
    return (
      <>
        <JlButton class="pin" variant="ghost" size="sm" icon="pin" hidden />
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
