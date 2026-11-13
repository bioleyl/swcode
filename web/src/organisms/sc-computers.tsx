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
  JlSelect,
} from '@syware/jlib';

import { COMPUTER_COLORS, ComputerColorSchema, THIS_COMPUTER } from '../../../shared/protocol/desktop.schema';
import { ScElement } from '../base/sc-element';
import {
  isLocalBase,
  LOCAL_ADDRESS_NOTE,
  reachableBaseOf,
  renewalLine,
  secondsUntilRenewal,
} from '../lib/pairing-view';
import { ScQrScanner } from '../molecules/sc-qr-scanner';
import { daemonOrigin } from '../services/access';
import { askComputers, onComputersAnswer } from '../services/computers';
import { daemonState } from '../services/daemon-status';
import { inDesktopApp } from '../services/desktop';
import { NO_CODE, newPairingCode, pairingLink } from '../services/pairing';
import { canScan } from '../services/qr-scan';
import { BASE_STYLES } from '../styles/shared';
import styles from './sc-computers.css?inline';

import type { MenuItem, SelectOption, Tone } from '@syware/jlib';
import type { Computer, ComputerColor, DesktopAnswer } from '../../../shared/protocol/desktop.schema';
import type { PairingCode } from '../../../shared/protocol/pairing.schema';
import type { ComputerAction, DesktopAnswerHandlers, DesktopAnswerMap } from '../types/desktop.types';

const PAIRING = 'Pairing…';
const ADDED = 'Added: open it to show its swcode here.';
const NO_ADDRESS = 'Type that computer’s address.';
const NO_NAME = 'Type the name this app should call that computer.';
const DEFAULT_COLOR = 'default';
const COLOR_LABELS: Readonly<Record<ComputerColor, string>> = {
  amber: 'Amber',
  blue: 'Blue',
  green: 'Green',
  orange: 'Orange',
  pink: 'Pink',
  red: 'Red',
  slate: 'Slate',
  teal: 'Teal',
  violet: 'Violet',
};
const COLOR_OPTIONS: SelectOption[] = [
  { label: 'Default', value: DEFAULT_COLOR },
  ...COMPUTER_COLORS.map((color) => ({ label: COLOR_LABELS[color], value: color })),
];
const COMPUTER_ACTIONS: MenuItem<ComputerAction>[] = [
  { icon: 'pencil', label: 'Edit', value: 'edit' },
  { divider: true },
  { danger: true, icon: 'trash', label: 'Forget', value: 'forget' },
];
const UNREACHABLE = 'not reachable';
const CONNECT_LEAD =
  'On the other computer’s swcode, or in the app a phone installed, open Settings → Computers and paste this link there, or type this address and this code, then Add. The code works once, and a new one comes every minute.';
const ADDRESS_LABEL = 'Address';
const CODE_LABEL = 'Pairing code';
const LINK_LABEL = 'Link';
// What the form asks for, by who keeps the computers: the desktop app reaches any address and takes a link with a
// token; a browser pairs with a code, from an address its page may talk to.
const ADD_TEXTS = {
  app: {
    addressHint: 'Its swcode listens on the network: SWCODE_HOST is 0.0.0.0 by default.',
    addressPlaceholder: 'gmktec, 192.168.1.20:4500, or a link with its token',
    codeHint:
      'That computer’s swcode shows one in its Settings → Computers topic, or run swcode pair there (swcode-desktop.exe --pair on Windows). A link with its token needs none.',
  },
  browser: {
    addressHint:
      'The link that computer’s swcode shows in Settings → Computers, or under its QR code; or its address.',
    addressPlaceholder: 'https://gmktec.tailnet.ts.net/?pair=…',
    codeHint: 'Only with an address: the link holds its code already.',
  },
} as const;
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
  // The computer the edit dialog is open on, until it closes.
  private _editing: Computer | undefined;
  private _code: PairingCode | undefined;
  // Why there is no code: what the daemon said, or that it does not answer.
  private _problem = NO_CODE;
  private _renewing = false;
  private _clock: ReturnType<typeof setInterval> | undefined;

  private readonly _handlers: DesktopAnswerHandlers = {
    computerRefused: (answer) => {
      this._pairing = false;
      this._refs.add.loading.set(false);
      if (this._editing) {
        this.refuseEdit(answer.message);
        return;
      }
      this.say(answer.message, 'danger');
    },
    computers: (answer) => {
      this.renderComputers(answer.computers, answer.current);
      if (this._editing) {
        this._refs.edit.close();
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
    computerUnreachable: () => {
      // The switch says that computer did not answer; the list says which ones are not reachable.
      askComputers({ type: 'listComputers' });
    },
  };

  private readonly _refs = this.useRefs((ref) => ({
    add: ref<JlButton>('.add'),
    address: ref<JlInput>('#computer-address'),
    addressField: ref<JlField>('.address-field'),
    code: ref<JlInput>('#computer-code'),
    codeField: ref<JlField>('.code-field'),
    computers: ref('.computers'),
    copyAddress: ref<JlCopyButton>('.copy-address'),
    copyCode: ref<JlCopyButton>('.copy-code'),
    copyLink: ref<JlCopyButton>('.copy-link'),
    edit: ref(JlDialog),
    editCancel: ref<JlButton>('.edit-cancel'),
    editColor: ref<JlSelect>('.edit-color'),
    editName: ref<JlInput>('.edit-name'),
    editProblem: ref<JlAlert>('.edit-problem'),
    editSave: ref<JlButton>('.edit-save'),
    expires: ref('.expires'),
    feedback: ref<JlAlert>('.feedback'),
    form: ref<HTMLFormElement>('form'),
    note: ref('.note'),
    pairAddress: ref('.address'),
    pairingCode: ref('.code'),
    pairLink: ref('.link'),
    scan: ref<JlButton>('.scan'),
    scanner: ref(ScQrScanner),
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
            <div class="pair">
              <span class="label">{LINK_LABEL}</span>
              <code class="link" />
              <JlCopyButton class="copy-link" />
            </div>
          </div>
          <p class="expires" />
          <p class="note" hidden>
            {LOCAL_ADDRESS_NOTE}
          </p>
        </section>
        <form>
          <h3>Add a computer</h3>
          <JlField class="address-field" label="Link or address">
            <JlInput id="computer-address" autocomplete="off" />
          </JlField>
          <JlField class="code-field" label="Pairing code">
            <JlInput id="computer-code" autocomplete="off" />
          </JlField>
          <JlAlert class="feedback" hidden />
          <div class="actions">
            <JlButton class="scan" icon="camera" label="Scan its QR code" hidden />
            <JlButton class="add" type="submit" variant="primary" label="Add" />
          </div>
        </form>
        <ScQrScanner />
        <JlDialog class="edit" heading="Edit computer" size="sm">
          <div class="edit-body">
            <JlField label="Name" hint="What this app calls that computer. Its address and token stay.">
              <JlInput class="edit-name" autocomplete="off" />
            </JlField>
            <JlField label="Colour" hint="The primary colour while this computer is shown.">
              <JlSelect class="edit-color" options={COLOR_OPTIONS} />
            </JlField>
            <JlAlert class="edit-problem" tone="danger" hidden />
          </div>
          <JlButton slot="actions" class="edit-cancel" label="Cancel" />
          <JlButton slot="actions" class="edit-save" variant="primary" label="Save" />
        </JlDialog>
      </div>
    );
  }

  templateCss(): string {
    return css`${BASE_STYLES}${styles}`;
  }

  onConnect(): void {
    this.renderForm();
    this._refs.scan.events.register('press', () => this._refs.scanner.scan());
    this._refs.scanner.events.register('found', (link) => this.addScanned(link));
    this.on(this._refs.form, 'submit', (event) => {
      event.preventDefault();
      this.add();
    });
    this._refs.add.events.register('press', () => this.add());
    this._refs.edit.events.register('close', () => this.stopEditing());
    this._refs.editCancel.events.register('press', () => this._refs.edit.close());
    this._refs.editSave.events.register('press', () => this.edit());
    onComputersAnswer((answer) => this.hear(answer), this.killSignal);
    askComputers({ type: 'listComputers' });
    if (this.active.get()) {
      this.startCode();
    }
  }

  onDisconnect(): void {
    this.stopClock();
  }

  /** What the form asks for where the computers are kept, and the scan button where the device can scan. */
  private renderForm(): void {
    const texts = ADD_TEXTS[inDesktopApp() ? 'app' : 'browser'];
    this._refs.addressField.hint.set(texts.addressHint);
    this._refs.address.placeholder.set(texts.addressPlaceholder);
    this._refs.codeField.hint.set(texts.codeHint);
    this._refs.scan.hidden = inDesktopApp() || !canScan();
  }

  /** The link a QR code held goes in the form, and is added at once: it carries its code. */
  private addScanned(link: string): void {
    this._refs.address.value.set(link);
    this._refs.code.value.set('');
    this.add();
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
    const base = reachableBaseOf(daemonState(), daemonOrigin());
    const code = this._code?.code ?? '';
    this._refs.pairAddress.textContent = base;
    this._refs.pairingCode.textContent = code;
    this._refs.copyAddress.value.set(base);
    this._refs.copyCode.value.set(code);
    this._refs.copyCode.hidden = code === '';
    const link = code === '' ? '' : pairingLink(base, code);
    this._refs.pairLink.textContent = link;
    this._refs.copyLink.value.set(link);
    this._refs.copyLink.hidden = code === '';
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
    askComputers({ address, code: this._refs.code.value.get().trim(), type: 'addComputer' });
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
      edit: () => this.openEdit(computer),
      forget: () => askComputers({ id: computer.id, type: 'forgetComputer' }),
    };
    actions[action]();
  }

  private openEdit(computer: Computer): void {
    this._editing = computer;
    this._refs.edit.heading.set(`Edit ${computer.name}`);
    this._refs.editName.value.set(computer.name);
    this._refs.editColor.value.set(computer.color ?? DEFAULT_COLOR);
    this.refuseEdit('');
    this._refs.edit.show();
  }

  private edit(): void {
    const computer = this._editing;
    if (!computer) {
      return;
    }
    const name = this._refs.editName.value.get().trim();
    if (name === '') {
      this.refuseEdit(NO_NAME);
      return;
    }
    const color = this.colorChosen();
    this._refs.editSave.loading.set(true);
    askComputers({ id: computer.id, name, type: 'editComputer', ...(color === undefined ? {} : { color }) });
  }

  private colorChosen(): ComputerColor | undefined {
    const chosen = ComputerColorSchema.safeParse(this._refs.editColor.value.get());
    return chosen.success ? chosen.data : undefined;
  }

  private stopEditing(): void {
    this._editing = undefined;
    this.refuseEdit('');
  }

  private refuseEdit(problem: string): void {
    this._refs.editSave.loading.set(false);
    this._refs.editProblem.description.set(problem);
    this._refs.editProblem.hidden = problem === '';
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
      open.events.register('press', () => askComputers({ id: computer.id, type: 'openComputer' }));
      row.append(open);
    }
    if (!local) {
      row.append(this.actionsNode(computer));
    }
    return row;
  }
}

ScComputers.register();
