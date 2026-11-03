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

const lineOf = (line: DiffLine): Node =>
  line.kind === 'gap' ? (
    <div class="gap">{line.text || GAP_MARK}</div>
  ) : (
    <div class="line" data-kind={line.kind}>
      <span class="number">{String(line.newLine ?? line.oldLine ?? '')}</span>
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

  private readonly _refs = this.useRefs((ref) => ({ body: ref('.body') }));

  templateHtml(): Node {
    return <div class="body" />;
  }

  templateCss(): string {
    return css`${BASE_STYLES}${styles}`;
  }
}

ScDiff.register();
