import { css } from '@jadis/core';
import { JlButton, JlDialog } from '@syware/jlib';
import { FitAddon } from '@xterm/addon-fit';
import { Terminal } from '@xterm/xterm';
import xtermStyles from '@xterm/xterm/css/xterm.css?inline';

import { ScElement } from '../base/sc-element';
import { BASE_STYLES } from '../styles/shared';
import styles from './sc-terminal.css?inline';

import type { Terminal as TerminalInfo, TerminalState } from '../../../shared/protocol/messages.schema';
import type { TerminalSize } from '../types/terminal.types';

const FONT_SIZE = 13;
const SCROLLBACK_LINES = 5000;
const FONT_FALLBACK = 'monospace';
const COLOR_FALLBACK = 'transparent';
const CURSOR_FALLBACK = '#4a9eff';
const INK_FALLBACK = '#d4d4d4';
const DARK_QUERY = '(prefers-color-scheme: dark)';

// How a terminal stands, in its dialog: a new state does not compile until it is here.
const STATE_TEXT: {
  readonly [Kind in TerminalState['kind']]: (state: Extract<TerminalState, { kind: Kind }>) => string;
} = {
  exited: (state) => `Exited (${state.code})`,
  running: () => 'Running',
  signalled: () => 'Stopped',
};

const stateText = (state: TerminalState): string =>
  (STATE_TEXT[state.kind] as (state: TerminalState) => string)(state);

// What xterm draws with: a token of the page as it is on screen, since xterm knows no CSS variables.
const token = (styles: CSSStyleDeclaration, name: string, fallback: string): string =>
  styles.getPropertyValue(name).trim() || fallback;

/**
 * A terminal of a project, in a dialog over the session: what the shell prints, and what the person types in it. The
 * page sends its input to the daemon and gives it what comes back; closing the dialog leaves the view, the dialog's
 * own close button ends the shell.
 */
export class ScTerminal extends ScElement {
  static readonly selector = 'sc-terminal';

  readonly events = this.useEvents<{ back: undefined; close: undefined; input: string; resize: TerminalSize }>();

  private _terminal: Terminal | undefined;
  private _fit: FitAddon | undefined;
  private _observer: ResizeObserver | undefined;
  private _theme: MutationObserver | undefined;
  private _open = false;
  // The terminal shown: a resize comes from the one view, and goes to the daemon by its id.
  private _shown = '';

  private readonly _refs = this.useRefs((ref) => ({
    close: ref<JlButton>('.close'),
    dialog: ref(JlDialog),
    screen: ref('.screen'),
  }));

  templateHtml(): Node {
    return (
      <JlDialog size="full">
        <div class="screen" />
        <JlButton slot="actions" class="close" icon="trash" label="Close terminal" variant="danger" />
      </JlDialog>
    );
  }

  templateCss(): string {
    return css`${BASE_STYLES}${xtermStyles}${styles}`;
  }

  onConnect(): void {
    this._refs.close.events.register('press', () => this.events.emit('close'));
    this._refs.dialog.events.register('close', () => this.closed());
    this.on(this._refs.screen, 'pointerdown', () => this._terminal?.focus());
    this._observer = new ResizeObserver(() => this.fit());
    this._observer.observe(this._refs.screen);
    this._theme = new MutationObserver(() => this.useTheme());
    this._theme.observe(document.documentElement, { attributeFilter: ['data-theme'] });
    window.matchMedia(DARK_QUERY).addEventListener('change', () => this.useTheme(), { signal: this.killSignal });
  }

  onDisconnect(): void {
    this._observer?.disconnect();
    this._observer = undefined;
    this._theme?.disconnect();
    this._theme = undefined;
  }

  /** Shows a terminal in its dialog: its name and how it stands; what it printed comes as the page writes it. */
  show(terminal: TerminalInfo): void {
    // Another terminal takes the view: what the last one printed goes, its own output coming after.
    if (this._shown !== '' && this._shown !== terminal.id && this._terminal) {
      this._terminal.reset();
    }
    this._shown = terminal.id;
    this._refs.dialog.heading.set(terminal.title);
    this._refs.dialog.description.set(`${stateText(terminal.state)} · ${terminal.cwd}`);
    if (!this._open) {
      this._open = true;
      this._refs.dialog.show();
    }
    this.useTheme();
    this.openTerminal();
    if (this._terminal) {
      // A shell that ended takes nothing more: what is typed stays in the view, unread by anyone.
      this._terminal.options.disableStdin = terminal.state.kind !== 'running';
    }
    requestAnimationFrame(() => {
      this.fit();
      this._terminal?.focus();
    });
  }

  /** Leaves the view: the shell keeps running for the next visit, and the session shows again. The screen keeps
   * what it holds, so the next terminal shown clears it. */
  hide(): void {
    if (this._open) {
      this._open = false;
      this._refs.dialog.close();
    }
  }

  /** How much of the view the shell's terminal should be, in characters. */
  size(): TerminalSize {
    return { columns: this._terminal?.cols ?? 0, rows: this._terminal?.rows ?? 0 };
  }

  /** The terminal from the start, then what it printed so far: what a page shows again after a reload. */
  replay(output: string): void {
    this._terminal?.reset();
    this.write(output);
  }

  /** What came from the shell, as the daemon sends it: base64 of bytes that are not always UTF-8. */
  write(data: string): void {
    const bytes = Uint8Array.from(atob(data), (character) => character.charCodeAt(0));
    this._terminal?.write(bytes);
  }

  /** The dialog closed by its own button or Escape: the page leaves the view, unless it asked for the close. */
  private closed(): void {
    const wasOpen = this._open;
    this._open = false;
    if (wasOpen) {
      this.events.emit('back');
    }
  }

  private openTerminal(): void {
    if (!this._terminal) {
      this.createTerminal();
      return;
    }
    this.fit();
  }

  private createTerminal(): void {
    const terminal = new Terminal({
      cursorBlink: true,
      fontFamily: token(getComputedStyle(this), '--jl-font-mono', FONT_FALLBACK),
      fontSize: FONT_SIZE,
      scrollback: SCROLLBACK_LINES,
      theme: this.theme(),
    });
    const fit = new FitAddon();
    terminal.loadAddon(fit);
    terminal.open(this._refs.screen);
    terminal.onData((data) => this.events.emit('input', data));
    terminal.onResize(({ cols, rows }) => this.events.emit('resize', { columns: cols, rows }));
    this._terminal = terminal;
    this._fit = fit;
    this.fit();
  }

  /** The colours of the page as it is now: xterm takes values, and the theme changes under it. */
  private theme(): { background: string; cursor: string; foreground: string; selectionBackground: string } {
    const tokens = getComputedStyle(this);
    return {
      // A colour, not transparent: xterm's own viewport is black, and a light theme would write black on it.
      background: token(tokens, '--jl-surface-2', COLOR_FALLBACK),
      cursor: token(tokens, '--jl-accent', CURSOR_FALLBACK),
      foreground: token(tokens, '--jl-text', INK_FALLBACK),
      selectionBackground: token(tokens, '--jl-accent-line', COLOR_FALLBACK),
    };
  }

  private useTheme(): void {
    if (this._terminal) {
      this._terminal.options.theme = this.theme();
    }
  }

  /** Sizes the terminal to its view, when the view has a size: closed, it has none. */
  private fit(): void {
    if (!this._fit || !this._terminal || !this._open || this._refs.screen.clientWidth === 0) {
      return;
    }
    this._fit.fit();
  }
}

ScTerminal.register();
