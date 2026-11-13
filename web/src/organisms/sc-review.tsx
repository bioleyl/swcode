import { createElement, css } from '@jadis/core';
import { JlAlert, JlButton, JlDialog, JlEmpty, JlTextarea } from '@syware/jlib';

import { MAX_COMMIT_MESSAGE } from '../../../shared/protocol/messages.schema';
import { ScElement } from '../base/sc-element';
import { canCommit, reviewDiffs, reviewSummary } from '../lib/review';
import { ScDiff } from '../molecules/sc-diff';
import { BASE_STYLES } from '../styles/shared';
import styles from './sc-review.css?inline';

import type { DiffDisplay, ToolDisplay } from '../../../shared/protocol/session.schema';

const MINUS = '−';
const READING = 'Reading what changed…';
const MESSAGE_ROWS = 2;
const MESSAGE_MAX_ROWS = 8;

/** What the daemon says of a workspace's uncommitted changes, as the dialog draws it. */
interface ReviewShown {
  branch: string;
  files: readonly ToolDisplay[];
  more?: number | undefined;
  problem?: string | undefined;
}

/** A changed file: where it is, how many lines it gained and lost, and the lines. */
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
 * What a session's workspace has uncommitted in its git repository, a diff a file, and a message to commit it all
 * with. `ask` asks the page for the changes (when it opens, and again after a commit), `commit` gives the message;
 * the page answers with `show` and `committed`.
 */
export class ScReview extends ScElement {
  static readonly selector = 'sc-review';

  readonly events = this.useEvents<{ ask: undefined; commit: string }>();

  // How many files the review shows changed, and whether a commit is under way: what the Commit button goes by.
  private _changed = 0;
  private _committing = false;

  private readonly _refs = this.useRefs((ref) => ({
    clean: ref(JlEmpty),
    close: ref<JlButton>('.close'),
    commit: ref<JlButton>('.commit'),
    dialog: ref(JlDialog),
    files: ref('.files'),
    message: ref(JlTextarea),
    more: ref('.more'),
    problem: ref(JlAlert),
    reading: ref('.reading'),
  }));

  templateHtml(): Node {
    return (
      <JlDialog heading="Changes" size="lg">
        <div class="body">
          <p class="reading">{READING}</p>
          <JlAlert tone="warn" hidden />
          <JlEmpty
            icon="check"
            heading="Nothing to commit"
            description="The workspace holds what its last commit holds."
            hidden
          />
          <div class="files" />
          <p class="more" hidden />
          <JlTextarea
            label="Commit message"
            placeholder="What this change does"
            rows={MESSAGE_ROWS}
            maxRows={MESSAGE_MAX_ROWS}
            maxlength={String(MAX_COMMIT_MESSAGE)}
            autosize
            hidden
          />
        </div>
        <JlButton slot="actions" class="close" label="Close" />
        <JlButton slot="actions" class="commit" variant="primary" label="Commit" disabled />
      </JlDialog>
    );
  }

  templateCss(): string {
    return css`${BASE_STYLES}${styles}`;
  }

  onConnect(): void {
    this._refs.message.events.register('input', () => this.renderCommit());
    this._refs.commit.events.register('press', () => this.commit());
    this._refs.close.events.register('press', () => this._refs.dialog.close());
  }

  /** Opens on the changes as they are now: the page is asked for them, and the dialog says so until they come. */
  open(): void {
    this._changed = 0;
    this._committing = false;
    this._refs.files.replaceChildren();
    this._refs.reading.hidden = false;
    this._refs.clean.hidden = true;
    this._refs.more.hidden = true;
    this._refs.message.hidden = true;
    this.say('');
    this._refs.dialog.description.set('');
    this.renderCommit();
    this._refs.dialog.show();
    this.events.emit('ask');
  }

  /** The changes the daemon read: the branch, a diff a file, or why there is nothing to show. */
  show(review: ReviewShown): void {
    const diffs = reviewDiffs(review.files);
    const more = review.more ?? 0;
    const problem = review.problem ?? '';
    this._changed = diffs.length;
    this._refs.reading.hidden = true;
    this._refs.files.replaceChildren(...diffs.map(fileNode));
    this._refs.clean.hidden = problem !== '' || diffs.length > 0;
    this._refs.more.hidden = more === 0;
    this._refs.more.textContent =
      more === 0 ? '' : `${more} more files changed: they are committed with the others.`;
    this._refs.message.hidden = diffs.length === 0;
    this._refs.dialog.description.set(problem === '' ? reviewSummary(review.branch, diffs.length, more) : '');
    this.say(problem);
    this.renderCommit();
  }

  /** How the commit went: git's own words when it refused, else the changes are read again. */
  committed(problem: string): void {
    this._committing = false;
    if (problem !== '') {
      this.say(problem);
      this.renderCommit();
      return;
    }
    this._refs.message.value.set('');
    this.renderCommit();
    this.events.emit('ask');
  }

  private commit(): void {
    const message = this._refs.message.value.get().trim();
    if (!canCommit(message, this._changed, this._committing)) {
      return;
    }
    this._committing = true;
    this.say('');
    this.renderCommit();
    this.events.emit('commit', message);
  }

  private say(problem: string): void {
    this._refs.problem.description.set(problem);
    this._refs.problem.hidden = problem === '';
  }

  private renderCommit(): void {
    this._refs.commit.loading.set(this._committing);
    this._refs.commit.disabled.set(!canCommit(this._refs.message.value.get(), this._changed, this._committing));
  }
}

ScReview.register();
