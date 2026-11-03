import { css } from '@jadis/core';

import { ScElement } from '../base/sc-element';
import { markElement } from '../lib/mark';
import { BASE_STYLES } from '../styles/shared';
import styles from './sc-brand.css?inline';

const DEFAULT_HEIGHT = 25;

/** The mark and the wordmark: "sw" in ink, "code" in the logo's blue; the wordmark is the `word` part. */
export class ScBrand extends ScElement {
  static readonly selector = 'sc-brand';

  readonly height = this.useChange(
    DEFAULT_HEIGHT,
    (value) => this._refs.mark.replaceChildren(markElement(value)),
    {
      attribute: 'height',
    }
  );

  private readonly _refs = this.useRefs((ref) => ({ mark: ref('.mark') }));

  templateHtml(): Node {
    return (
      <>
        <span class="mark" />
        <span class="word" attrs={{ part: 'word' }}>
          sw<span class="code">code</span>
        </span>
      </>
    );
  }

  templateCss(): string {
    return css`${BASE_STYLES}${styles}`;
  }
}

ScBrand.register();
