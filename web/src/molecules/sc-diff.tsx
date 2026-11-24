import { css } from '@jadis/core';

import { ScElement } from '../base/sc-element';
import { BASE_STYLES } from '../styles/shared';
import styles from './sc-diff.css?inline';

import type { DiffLine } from '../../../shared/protocol/session.schema';

const SIGNS: Readonly<Record<DiffLine['kind'], string>> = {
  add: '+',
  context: ' ',
  gap: '',
  remove: '−',
};

const GAP_MARK = '⋯';

/** The line's number in the file the change is of: the new one, the old one where a line was removed. */
const numberOf = (line: DiffLine): number | undefined => line.newLine ?? line.oldLine;

const lineOf = (line: DiffLine): Node =>
  line.kind === 'gap' ? (
    <div class="gap">{line.text || GAP_MARK}</div>
  ) : (
    <div class="line" data-kind={line.kind} data-line={numberOf(line)}>
      <span class="number">{String(numberOf(line) ?? '')}</span>
      <span class="sign">{SIGNS[line.kind]}</span>
      <span class="text">{line.text}</span>
    </div>
  );

/** The lines of a change to a file: added in blue, removed in red, the lines around them plain. */
export class ScDiff extends ScElement {
  static readonly selector = 'sc-diff';

  readonly lines = this.useChange<DiffLine[]>([], (lines) =>
    this._refs.body.replaceChildren(...lines.map(lineOf))
  );

  /** The person clicked a line: the host is asked to open the file at it. */
  readonly events = this.useEvents<{ open: { line: number } }>();

  private readonly _refs = this.useRefs((ref) => ({ body: ref('.body') }));

  templateHtml(): Node {
    return <div class="body" />;
  }

  templateCss(): string {
    return css`${BASE_STYLES}${styles}`;
  }

  onConnect(): void {
    this.on(this._refs.body, 'click', (event) => this.openLine(event));
  }

  /** The line the click landed on, read off the row it was drawn in; a gap has none, so it asks for nothing. */
  private openLine(event: MouseEvent): void {
    const row = (event.target as HTMLElement | null)?.closest<HTMLElement>('[data-line]');
    const line = Number(row?.dataset.line);
    if (row && Number.isFinite(line)) {
      this.events.emit('open', { line });
    }
  }
}

ScDiff.register();
