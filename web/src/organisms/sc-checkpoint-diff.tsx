import { createElement, css } from '@jadis/core';
import { JlDialog } from '@syware/jlib';

import { ScElement } from '../base/sc-element';
import { ScDiff } from '../molecules/sc-diff';
import { BASE_STYLES } from '../styles/shared';
import styles from './sc-checkpoint-diff.css?inline';

import type { DiffDisplay } from '../../../shared/protocol/session.schema';

const MINUS = '−';

/** A file of the change: where it is, and how many lines it gained and lost. */
const fileNode = (diff: DiffDisplay): Node => {
  const lines = createElement(ScDiff);
  lines.lines.set(diff.lines);
  return (
    <div class="file">
      <p class="head">
        <span class="path">{diff.path}</span>
        <span class="counts">
          +{diff.added} {MINUS}
          {diff.removed}
        </span>
      </p>
      {lines}
    </div>
  );
};

/**
 * The files a turn changed, a diff each: what they held before the turn, and what they hold now, for a person to see
 * what "Restore" would put back.
 */
export class ScCheckpointDiff extends ScElement {
  static readonly selector = 'sc-checkpoint-diff';

  private readonly _refs = this.useRefs((ref) => ({
    dialog: ref(JlDialog),
    files: ref('.files'),
  }));

  templateHtml(): Node {
    return (
      <JlDialog
        description="Each file as it was before this turn, and as it is now."
        heading="Changes of this turn"
        size="lg">
        <div class="files" />
      </JlDialog>
    );
  }

  templateCss(): string {
    return css`${BASE_STYLES}${styles}`;
  }

  /** The turn's files, drawn in the dialog, which opens. */
  show(diffs: readonly DiffDisplay[]): void {
    this._refs.files.replaceChildren(...diffs.map(fileNode));
    this._refs.dialog.show();
  }
}

ScCheckpointDiff.register();
