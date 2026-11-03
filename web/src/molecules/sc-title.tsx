import { css } from '@jadis/core';

import { ScElement } from '../base/sc-element';
import { BASE_STYLES } from '../styles/shared';
import styles from './sc-title.css?inline';

const ENTER = 'Enter';
const ESCAPE = 'Escape';
const MAX_TITLE_LENGTH = 120;

/** A session's title, renamed in place: a click turns it into a field; Enter or leaving the field keeps it. */
export class ScTitle extends ScElement {
  static readonly selector = 'sc-title';

  readonly events = this.useEvents<{ rename: string }>();

  readonly value = this.useChange('', (value) => {
    this._refs.show.textContent = value;
    this._refs.show.title = `Rename “${value}”`;
  });
  /** A title that cannot be renamed, such as a session the daemon does not have. */
  readonly fixed = this.useChange(false, (value) => {
    this._refs.show.disabled = value;
  });

  private readonly _refs = this.useRefs((ref) => ({
    edit: ref<HTMLInputElement>('input'),
    show: ref<HTMLButtonElement>('button'),
    title: ref('h1'),
  }));

  templateHtml(): Node {
    return (
      <>
        <h1>
          <button type="button" />
        </h1>
        <input hidden maxLength={MAX_TITLE_LENGTH} aria-label="Session title" />
      </>
    );
  }

  templateCss(): string {
    return css`${BASE_STYLES}${styles}`;
  }

  onConnect(): void {
    this.on(this._refs.show, 'click', () => this.edit());
    this.on(this._refs.edit, 'keydown', (event) => this.keyDown(event));
    this.on(this._refs.edit, 'blur', () => this.save());
  }

  private keyDown(event: KeyboardEvent): void {
    if (event.key === ESCAPE) {
      this.close();
      return;
    }
    if (event.key !== ENTER) {
      return;
    }
    event.preventDefault();
    this.save();
  }

  private edit(): void {
    this._refs.edit.value = this.value.get();
    this._refs.title.hidden = true;
    this._refs.edit.hidden = false;
    this._refs.edit.focus();
    this._refs.edit.select();
  }

  /** Keeps a new, non-empty title; Enter then the blur that follows save once. */
  private save(): void {
    if (this._refs.edit.hidden) {
      return;
    }
    const title = this._refs.edit.value.trim();
    this.close();
    if (title === '' || title === this.value.get()) {
      return;
    }
    this.events.emit('rename', title);
  }

  private close(): void {
    this._refs.edit.hidden = true;
    this._refs.title.hidden = false;
  }
}

ScTitle.register();
