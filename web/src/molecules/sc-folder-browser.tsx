import { css } from '@jadis/core';
import { JlButton, JlIcon } from '@syware/jlib';

import { ScElement } from '../base/sc-element';
import { childPath } from '../lib/folders';
import { BASE_STYLES } from '../styles/shared';
import styles from './sc-folder-browser.css?inline';

import type { FolderView } from '../types/project.types';

const ENTER = 'Enter';
const LOADING = 'Looking…';
const EMPTY = 'No folders inside.';

/**
 * The folders of the daemon's computer, one at a time: its subfolders to go into, the way up, and its path, which
 * can be typed too. It asks for nothing itself: `go` says where to go, `show` draws what the daemon answered.
 */
export class ScFolderBrowser extends ScElement {
  static readonly selector = 'sc-folder-browser';

  readonly events = this.useEvents<{ go: string }>();

  private _view: FolderView | undefined;

  private readonly _refs = this.useRefs((ref) => ({
    folders: ref('ul'),
    path: ref<HTMLInputElement>('input'),
    state: ref('.state'),
    up: ref<JlButton>('.up'),
  }));

  /** The folder shown, once the daemon listed it. */
  get path(): string | undefined {
    return this._view?.problem === undefined ? this._view?.path : undefined;
  }

  templateHtml(): Node {
    return (
      <>
        <div class="where">
          <JlButton class="up" icon="corner-left-up" hint="Parent folder" variant="ghost" size="sm" />
          <label class="sr-only" htmlFor="folder-path">
            Folder
          </label>
          <input id="folder-path" autocomplete="off" spellcheck={false} />
        </div>
        <ul aria-label="Folders inside" />
        <p class="state" />
      </>
    );
  }

  templateCss(): string {
    return css`${BASE_STYLES}${styles}`;
  }

  onConnect(): void {
    this._refs.up.events.register('press', () => this.goUp());
    this.on(this._refs.path, 'keydown', (event) => this.typed(event));
    this.on(this._refs.folders, 'click', (event) => this.picked(event));
  }

  /** Waits for the daemon's answer about a folder. */
  loading(): void {
    this._refs.state.textContent = LOADING;
    this._refs.folders.replaceChildren();
    this._refs.up.disabled.set(true);
  }

  show(view: FolderView): void {
    this._view = view;
    this._refs.path.value = view.path;
    this._refs.up.disabled.set(view.parent === undefined);
    this._refs.folders.replaceChildren(
      ...view.folders.map((name) => (
        <li>
          <button type="button" data-name={name}>
            <JlIcon name="folder" size={16} />
            <span class="name">{name}</span>
            <JlIcon name="chevron-right" size={14} />
          </button>
        </li>
      ))
    );
    this._refs.state.textContent = view.problem ?? (view.folders.length === 0 ? EMPTY : '');
    this.toggleClass('refused', view.problem !== undefined);
  }

  private goUp(): void {
    const parent = this._view?.parent;
    if (parent === undefined) {
      return;
    }
    this.events.emit('go', parent);
  }

  private typed(event: KeyboardEvent): void {
    const path = this._refs.path.value.trim();
    if (event.key !== ENTER || path === '') {
      return;
    }
    event.preventDefault();
    this.events.emit('go', path);
  }

  private picked(event: MouseEvent): void {
    const name = event.target instanceof Element ? event.target.closest('button')?.dataset.name : undefined;
    if (name === undefined || this._view === undefined) {
      return;
    }
    this.events.emit('go', childPath(this._view.path, name));
  }
}

ScFolderBrowser.register();
