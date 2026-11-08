import { css } from '@jadis/core';
import { JlIcon, JlSelect } from '@syware/jlib';

import { THIS_COMPUTER } from '../../../shared/protocol/desktop.schema';
import { ScElement } from '../base/sc-element';
import { applyComputerColor } from '../services/computer-color';
import { askDesktop, inDesktopApp, onDesktopAnswer } from '../services/desktop';
import { navigate, settingsPath } from '../services/navigation';
import { BASE_STYLES, PILL_SELECT_STYLES } from '../styles/shared';
import styles from './sc-computer-switch.css?inline';

import type { SelectOption } from '@syware/jlib';
import type { Computer } from '../../../shared/protocol/desktop.schema';
import type { IconName } from '../lib/icons';

const MANAGE = 'manage';
const MANAGE_LABEL = 'Manage computers…';
const COMPUTERS_TOPIC = 'computers';
const UNREACHABLE = ' (not reachable)';
const THIS_COMPUTER_NAME = 'This computer';
const LOOPBACK_NAMES: ReadonlySet<string> = new Set(['localhost', '127.0.0.1', '[::1]']);
const PLACE_ICONS: Readonly<Record<'local' | 'remote', IconName>> = { local: 'laptop', remote: 'server' };

/** The computers the app knows, then the way to the settings that add them. */
const optionsOf = (computers: readonly Computer[]): SelectOption[] => [
  ...computers.map((computer) => ({
    label: computer.reachable ? computer.name : `${computer.name}${UNREACHABLE}`,
    value: computer.id,
  })),
  { label: MANAGE_LABEL, value: MANAGE },
];

/**
 * Where swcode works, at the top of the sidebar: this computer, or another one, tinted and marked remote so it is
 * never mistaken for this one. In the desktop app it switches between the computers the app knows, and leads to
 * the settings that add them; in a browser it names the machine the daemon runs on.
 */
export class ScComputerSwitch extends ScElement {
  static readonly selector = 'sc-computer-switch';

  /** The machine the daemon runs on, as its status names it: what a browser shows. */
  readonly computer = this.useChange('', () => this.renderBrowser());

  private _current: string = THIS_COMPUTER;

  private readonly _refs = this.useRefs((ref) => ({
    badge: ref('.badge'),
    icon: ref<JlIcon>('.place-icon'),
    name: ref('.name'),
    select: ref(JlSelect),
  }));

  templateHtml(): Node {
    return (
      <div class="where">
        <JlIcon class="place-icon" name="laptop" size={16} />
        <span class="name" />
        <JlSelect class="pill-select" label="Computer" hidden />
        <span class="badge" hidden>
          Remote
        </span>
      </div>
    );
  }

  templateCss(): string {
    return css`${BASE_STYLES}${PILL_SELECT_STYLES}${styles}`;
  }

  onConnect(): void {
    if (!inDesktopApp()) {
      this.renderBrowser();
      return;
    }
    // In the desktop app, the pill chooses among the computers; in a browser it only names the place.
    this._refs.name.hidden = true;
    this._refs.select.hidden = false;
    this._refs.select.events.register('change', (value) => this.choose(value));
    onDesktopAnswer((answer) => {
      if (answer.type === 'computers') {
        this.renderComputers(answer.computers, answer.current);
      }
    }, this.killSignal);
    askDesktop({ type: 'listComputers' });
  }

  private renderComputers(computers: readonly Computer[], current: string): void {
    this._current = current;
    this._refs.select.options.set(optionsOf(computers));
    this._refs.select.value.set(current);
    this.renderPlace(current !== THIS_COMPUTER);
    applyComputerColor(computers.find((computer) => computer.id === current)?.color);
  }

  /** A browser knows the daemon's machine, and whether the page came from this one. */
  private renderBrowser(): void {
    if (inDesktopApp()) {
      return;
    }
    const remote = !LOOPBACK_NAMES.has(location.hostname);
    this._refs.name.textContent = remote ? this.computer.get() || location.hostname : THIS_COMPUTER_NAME;
    this.renderPlace(remote);
  }

  private renderPlace(remote: boolean): void {
    this.toggleClass('remote', remote);
    this._refs.badge.hidden = !remote;
    this._refs.icon.name.set(PLACE_ICONS[remote ? 'remote' : 'local']);
  }

  private choose(value: string): void {
    if (value === MANAGE) {
      // Not a place to be: the pill goes back to the computer in use.
      this._refs.select.value.set(this._current);
      navigate(settingsPath(COMPUTERS_TOPIC));
      return;
    }
    if (value === this._current) {
      return;
    }
    askDesktop({ id: value, type: 'openComputer' });
  }
}

ScComputerSwitch.register();
