import { createElement, css } from '@jadis/core';
import {
  JlAlert,
  JlBadge,
  JlButton,
  JlCopyButton,
  JlDialog,
  JlField,
  JlInput,
  JlListItem,
  JlMenu,
} from '@syware/jlib';

import { THIS_COMPUTER } from '../../../shared/protocol/desktop.schema';
import { ScElement } from '../base/sc-element';
import {
  isLocalBase,
  LOCAL_ADDRESS_NOTE,
  reachableBaseOf,
  renewalLine,
  secondsUntilRenewal,
} from '../lib/pairing-view';
import { daemonState } from '../services/daemon-status';
import { askDesktop, inDesktopApp, onDesktopAnswer } from '../services/desktop';
import { NO_CODE, newPairingCode } from '../services/pairing';
import { BASE_STYLES } from '../styles/shared';
import styles from './sc-computers.css?inline';

import type { MenuItem, Tone } from '@syware/jlib';
import type { Computer, DesktopAnswer } from '../../../shared/protocol/desktop.schema';
import type { PairingCode } from '../../../shared/protocol/pairing.schema';
import type { ComputerAction, DesktopAnswerHandlers, DesktopAnswerMap } from '../types/desktop.types';

const PAIRING = 'Pairing…';
const ADDED = 'Added: open it to show its swcode here.';
const NO_ADDRESS = 'Type that computer’s address.';
const NO_NAME = 'Type the name this app should call that computer.';
const COMPUTER_ACTIONS: MenuItem<ComputerAction>[] = [
  { icon: 'pencil', label: 'Rename', value: 'rename' },
  { divider: true },
  { danger: true, icon: 'trash', label: 'Forget', value: 'forget' },
];
const NOT_IN_APP =
  'Only swcode’s desktop app shows other computers. In a browser, open the other computer’s swcode at its own address.';
const UNREACHABLE = 'not reachable';
const CONNECT_LEAD =
  'On the other computer’s swcode, open Settings → Computers, type this address and this code there, then Add. The code works once, and a new one comes every minute.';
const ADDRESS_LABEL = 'Address';
const CODE_LABEL = 'Pairing code';
const MS_PER_SECOND = 1000;

/**
 * The settings' Computers: the computers the desktop app's window shows, this one and the others paired with it,
 * whose swcode works on while this one is off. The app keeps them and their tokens: the page only asks it.
 */
export class ScComputers extends ScElement {
  static readonly selector = 'sc-computers';

  /** Whether its topic is shown: codes are asked for while they can be read. */
  readonly active = this.useChange(false, (active) => (active ? this.startCode() : this.stopClock()));

  // Set from asking the app to pair until it answers.
  private _pairing = false;
  // The computer the rename dialog is open on, until it closes.
  private _renaming: Computer | undefined;
  private _code: PairingCode | undefined;
  // Why there is no code: what the daemon said, or that it does not answer.
  private _problem = NO_CODE;
  private _renewing = false;
  private _clock: ReturnType<typeof setInterval> | undefined;

  private readonly _handlers: DesktopAnswerHandlers = {
    computerRefused: (answer) => {
      this._pairing = false;
      this._refs.add.loading.set(false);
      if (this._renaming) {
        this.refuseRename(answer.message);
        return;
      }
      this.say(answer.message, 'danger');
    },
    computers: (answer) => {
      this.renderComputers(answer.computers, answer.current);
      if (this._renaming) {
        this._refs.rename.close();
      }
      if (!this._pairing) {
        return;
      }
      this._pairing = false;
      this._refs.add.loading.set(false);
      this.say(ADDED, 'ok');
      this._refs.address.value.set('');
      this._refs.code.value.set('');
    },
  };

  private readonly _refs = this.useRefs((ref) => ({
    add: ref<JlButton>('.add'),
    address: ref<JlInput>('#computer-address'),
    code: ref<JlInput>('#computer-code'),
    computers: ref('.computers'),
    copyAddress: ref<JlCopyButton>('.copy-address'),
    copyCode: ref<JlCopyButton>('.copy-code'),
    expires: ref('.expires'),
    feedback: ref<JlAlert>('.feedback'),
    form: ref<HTMLFormElement>('form'),
    note: ref('.note'),
    notInApp: ref('.not-in-app'),
    pairAddress: ref('.address'),
    pairingCode: ref('.code'),
    rename: ref(JlDialog),
    renameCancel: ref<JlButton>('.rename-cancel'),
    renameName: ref<JlInput>('.rename-name'),
    renameProblem: ref<JlAlert>('.rename-problem'),
    renameSave: ref<JlButton>('.rename-save'),
  }));

  templateHtml(): Node {
    return (
      <div class="computers-topic">
        <p class="lead">
          The window shows swcode on this computer, or on another one paired with this app once. Another computer’s
          swcode works on while this one is off, and its sessions notify you here.
        </p>
        <div class="computers" />
        <section class="connect-from">
          <h3>Connect from another computer</h3>
          <p class="lead">{CONNECT_LEAD}</p>
          <div class="pairing">
            <div class="pair">
              <span class="label">{ADDRESS_LABEL}</span>
              <code class="address" />
              <JlCopyButton class="copy-address" />
            </div>
            <div class="pair">
              <span class="label">{CODE_LABEL}</span>
              <code class="code" />
              <JlCopyButton class="copy-code" />
            </div>
          </div>
          <p class="expires" />
          <p class="note" hidden>
            {LOCAL_ADDRESS_NOTE}
          </p>
        </section>
        <form>
          <h3>Add a computer</h3>
          <JlField label="Address" hint="Its swcode listens on the network: SWCODE_HOST is 0.0.0.0 by default.">
            <JlInput
              id="computer-address"
              placeholder="gmktec, 192.168.1.20:4500, or a link with its token"
              autocomplete="off"
            />
          </JlField>
          <JlField
            label="Pairing code"
            hint="That computer’s swcode shows one in its Settings → Computers topic, or run swcode pair there (swcode-desktop.exe --pair on Windows). A link with its token needs none.">
            <JlInput id="computer-code" autocomplete="off" />
          </JlField>
          <JlAlert class="feedback" hidden />
          <div class="actions">
            <JlButton class="add" type="submit" variant="primary" label="Add" />
          </div>
        </form>
        <JlDialog class="rename" heading="Rename computer" size="sm">
          <div class="rename-body">
            <JlField label="Name" hint="What this app calls that computer. Its address and token stay.">
              <JlInput class="rename-name" autocomplete="off" />
            </JlField>
            <JlAlert class="rename-problem" tone="danger" hidden />
          </div>
          <JlButton slot="actions" class="rename-cancel" label="Cancel" />
          <JlButton slot="actions" class="rename-save" variant="primary" label="Save" />
        </JlDialog>
        <p class="not-in-app" hidden>
          {NOT_IN_APP}
        </p>
      </div>
    );
  }

  templateCss(): string {
    return css`${BASE_STYLES}${styles}`;
  }

  onConnect(): void {
    const inApp = inDesktopApp();
    this._refs.notInApp.hidden = inApp;
    this._refs.form.hidden = !inApp;
    this.on(this._refs.form, 'submit', (event) => {
      event.preventDefault();
      this.add();
    });
    this._refs.add.events.register('press', () => this.add());
    this._refs.rename.events.register('close', () => this.stopRenaming());
    this._refs.renameCancel.events.register('press', () => this._refs.rename.close());
    this._refs.renameSave.events.register('press', () => this.rename());
    onDesktopAnswer((answer) => this.hear(answer), this.killSignal);
    askDesktop({ type: 'listComputers' });
    if (this.active.get()) {
      this.startCode();
    }
  }

  onDisconnect(): void {
    this.stopClock();
  }

  /** A first code, renewed every minute for as long as the topic is shown. */
  private startCode(): void {
    if (!this.isConnected) {
      return;
    }
    this._code = undefined;
    this.renderPairing();
    void this.renew();
    this.startClock();
  }

  private startClock(): void {
    this.stopClock();
    this._clock = setInterval(() => this.tick(), MS_PER_SECOND);
  }

  private stopClock(): void {
    clearInterval(this._clock);
    this._clock = undefined;
  }

  private tick(): void {
    const code = this._code;
    if (!code || secondsUntilRenewal(code.expiresAt, Date.now()) === 0) {
      void this.renew();
    }
    this.renderExpiry();
  }

  private async renew(): Promise<void> {
    if (this._renewing) {
      return;
    }
    this._renewing = true;
    try {
      const code = await newPairingCode();
      this._code = code.ok ? code.answer : undefined;
      this._problem = code.ok ? '' : code.problem;
    } finally {
      this._renewing = false;
    }
    this.renderPairing();
  }

  private renderPairing(): void {
    const base = reachableBaseOf(daemonState(), location.origin);
    const code = this._code?.code ?? '';
    this._refs.pairAddress.textContent = base;
    this._refs.pairingCode.textContent = code;
    this._refs.copyAddress.value.set(base);
    this._refs.copyCode.value.set(code);
    this._refs.copyCode.hidden = code === '';
    this._refs.note.hidden = !isLocalBase(base);
    this.renderExpiry();
  }

  private renderExpiry(): void {
    const code = this._code;
    this._refs.expires.textContent = code
      ? renewalLine(secondsUntilRenewal(code.expiresAt, Date.now()))
      : this._problem;
  }

  private hear(answer: DesktopAnswer): void {
    this.handle(answer.type, answer);
  }

  private handle<Type extends keyof DesktopAnswerMap>(type: Type, answer: DesktopAnswerMap[Type]): void {
    this._handlers[type](answer);
  }

  private say(text: string, tone: Tone): void {
    this._refs.feedback.tone.set(tone);
    this._refs.feedback.description.set(text);
    this._refs.feedback.hidden = text === '';
  }

  private add(): void {
    if (this._pairing) {
      return;
    }
    const address = this._refs.address.value.get().trim();
    if (address === '') {
      this.say(NO_ADDRESS, 'danger');
      return;
    }
    this._pairing = true;
    this._refs.add.loading.set(true);
    this.say(PAIRING, 'info');
    askDesktop({ address, code: this._refs.code.value.get().trim(), type: 'addComputer' });
  }

  /** The actions of a paired computer, in its row's More menu: the menu keeps its clicks to itself. */
  private actionsNode(computer: Computer): Node {
    const menu = createElement(JlMenu<ComputerAction>, {
      attrs: { slot: 'end' },
      props: { items: COMPUTER_ACTIONS, label: `Actions for ${computer.name}` },
    });
    menu.append(
      createElement(JlButton, {
        attrs: { slot: 'trigger' },
        props: { hint: 'More actions', icon: 'ellipsis', size: 'sm', variant: 'ghost' },
      })
    );
    menu.events.register('select', (action) => this.act(action, computer));
    return menu;
  }

  private act(action: ComputerAction, computer: Computer): void {
    const actions: Record<ComputerAction, () => void> = {
      forget: () => askDesktop({ id: computer.id, type: 'forgetComputer' }),
      rename: () => this.openRename(computer),
    };
    actions[action]();
  }

  private openRename(computer: Computer): void {
    this._renaming = computer;
    this._refs.rename.heading.set(`Rename ${computer.name}`);
    this._refs.renameName.value.set(computer.name);
    this.refuseRename('');
    this._refs.rename.show();
  }

  private rename(): void {
    const computer = this._renaming;
    if (!computer) {
      return;
    }
    const name = this._refs.renameName.value.get().trim();
    if (name === '') {
      this.refuseRename(NO_NAME);
      return;
    }
    this._refs.renameSave.loading.set(true);
    askDesktop({ id: computer.id, name, type: 'renameComputer' });
  }

  private stopRenaming(): void {
    this._renaming = undefined;
    this.refuseRename('');
  }

  private refuseRename(problem: string): void {
    this._refs.renameSave.loading.set(false);
    this._refs.renameProblem.description.set(problem);
    this._refs.renameProblem.hidden = problem === '';
  }

  private renderComputers(computers: readonly Computer[], current: string): void {
    this._refs.computers.replaceChildren(...computers.map((computer) => this.computerNode(computer, current)));
  }

  /** A computer, and its actions: each control keeps its clicks to itself, so it knows its computer. */
  private computerNode(computer: Computer, current: string): Node {
    const shown = computer.id === current;
    const local = computer.id === THIS_COMPUTER;
    const row = createElement(JlListItem, {
      attrs: { class: shown ? 'computer shown' : 'computer' },
      props: {
        description: computer.reachable ? computer.address : `${computer.address} · ${UNREACHABLE}`,
        heading: computer.name,
        icon: local ? 'laptop' : 'server',
        selected: shown,
      },
    });
    if (shown) {
      row.append(
        createElement(JlBadge, { attrs: { slot: 'end' }, props: { label: 'Shown here', tone: 'accent' } })
      );
    }
    if (!shown) {
      const open = createElement(JlButton, {
        attrs: { class: 'open', slot: 'end' },
        props: { label: 'Open', size: 'sm' },
      });
      open.events.register('press', () => askDesktop({ id: computer.id, type: 'openComputer' }));
      row.append(open);
    }
    if (!local) {
      row.append(this.actionsNode(computer));
    }
    return row;
  }
}

ScComputers.register();
