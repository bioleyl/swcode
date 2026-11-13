import { css } from '@jadis/core';
import { JlIcon, JlSelect, toast } from '@syware/jlib';

import { THIS_COMPUTER } from '../../../shared/protocol/desktop.schema';
import { ScElement } from '../base/sc-element';
import { applyComputerColor } from '../services/computer-color';
import { askComputers, onComputersAnswer } from '../services/computers';
import { inDesktopApp } from '../services/desktop';
import { navigate, settingsPath } from '../services/navigation';
import { BASE_STYLES, PILL_SELECT_STYLES } from '../styles/shared';
import styles from './sc-computer-switch.css?inline';

import type { SelectOption } from '@syware/jlib';
import type { Computer, DesktopAnswer } from '../../../shared/protocol/desktop.schema';
import type { IconName } from '../lib/icons';
import type { ComputerAnswers, DesktopAnswerMap } from '../types/desktop.types';

const MANAGE = 'manage';
const MANAGE_LABEL = 'Manage computers…';
const COMPUTERS_TOPIC = 'computers';
const UNREACHABLE = ' (not reachable)';
const DID_NOT_ANSWER = 'did not answer';
const CHECK_SWCODE = 'Is swcode running there, and listening on the network?';
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
 * never mistaken for this one. It switches between the computers the app knows (the desktop app's, or the ones
 * this browser paired with), and leads to the settings that add them.
 */
export class ScComputerSwitch extends ScElement {
  static readonly selector = 'sc-computer-switch';

  private _current: string = THIS_COMPUTER;

  // The answers about the computers this switch shows; a refused one comes from the settings' form.
  private readonly _answers: ComputerAnswers = {
    computers: (answer) => this.renderComputers(answer.computers, answer.current),
    computerUnreachable: (answer) => this.refuse(answer),
  };

  private readonly _refs = this.useRefs((ref) => ({
    badge: ref('.badge'),
    icon: ref<JlIcon>('.place-icon'),
    select: ref(JlSelect),
  }));

  templateHtml(): Node {
    return (
      <div class="where">
        <JlIcon class="place-icon" name="laptop" size={16} />
        <JlSelect class="pill-select" label="Computer" />
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
    this._refs.select.events.register('change', (value) => this.choose(value));
    onComputersAnswer((answer) => this.hear(answer), this.killSignal);
    askComputers({ type: 'listComputers' });
  }

  private hear(answer: DesktopAnswer): void {
    if (answer.type === 'computerRefused') {
      return;
    }
    this.handle(answer.type, answer);
  }

  private handle<Type extends keyof ComputerAnswers>(type: Type, answer: DesktopAnswerMap[Type]): void {
    this._answers[type](answer);
  }

  private renderComputers(computers: readonly Computer[], current: string): void {
    this._current = current;
    this._refs.select.options.set(optionsOf(computers));
    this._refs.select.value.set(current);
    // In a browser, the computer that served the page is another one too, unless the page was opened on it.
    const servedFromElsewhere = !inDesktopApp() && !LOOPBACK_NAMES.has(location.hostname);
    this.renderPlace(current !== THIS_COMPUTER || servedFromElsewhere);
    applyComputerColor(computers.find((computer) => computer.id === current)?.color);
  }

  /** A computer that is off cannot be shown: the pill goes back to the one the window is on, and says which. */
  private refuse(answer: DesktopAnswerMap['computerUnreachable']): void {
    this._refs.select.value.set(this._current);
    toast({ heading: `${answer.name} ${DID_NOT_ANSWER}`, text: CHECK_SWCODE, tone: 'danger' });
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
    askComputers({ id: value, type: 'openComputer' });
  }
}

ScComputerSwitch.register();
