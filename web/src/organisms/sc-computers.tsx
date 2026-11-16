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
import { newPairNumber } from '../lib/pair-number';
import { DEFAULT_PORT, pairingOrigin } from '../lib/remote-computers';
import { ScQrScanner } from '../molecules/sc-qr-scanner';
import { askComputers, onComputersAnswer } from '../services/computers';
import { inDesktopApp } from '../services/desktop';
import { deviceName, waitForPairing } from '../services/pairing';
import { canScan } from '../services/qr-scan';
import { BASE_STYLES } from '../styles/shared';
import styles from './sc-computers.css?inline';

import type { MenuItem, SelectOption, Tone } from '@syware/jlib';
import type { Computer, ComputerColor, DesktopAnswer } from '../../../shared/protocol/desktop.schema';
import type { ComputerAction, DesktopAnswerHandlers, DesktopAnswerMap } from '../types/desktop.types';

const PAIR_COMMAND = 'swcode pair ';
const WAITING = 'Waiting for that computer to take the number…';
const WAIT_LEAD = 'On that computer, run this where its swcode runs. The number waits a few minutes.';
const ADDED = 'Added: open it to show its swcode here.';
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
  'Add another computer here, then open it to show its swcode: its sessions and projects work on while this one is off, and its sessions notify you here.';
// What the form asks for, by who keeps the computers: the desktop app reaches any address, a browser only those its
// page may talk to.
const ADDRESS_TEXTS = {
  app: {
    hint: 'Its swcode has to listen on the network (its Settings → Network), or have an address on a tailnet.',
    placeholder: 'gmktec, 192.168.1.20, or its tailnet address',
  },
  browser: {
    hint: 'Its name or IP address, or the address this page came from if that computer serves it. It has to be HTTPS when this page is.',
    placeholder: 'https://gmktec.tailnet.ts.net',
  },
} as const;

/**
 * The settings' Computers: the computers the desktop app's window shows, this one and the others paired with it,
 * whose swcode works on while this one is off. The app keeps them and their tokens: the page only asks it.
 */
export class ScComputers extends ScElement {
  static readonly selector = 'sc-computers';

  // Set from asking to pair until the app takes the computer.
  private _adding = false;
  // The computer the edit dialog is open on, until it closes.
  private _editing: Computer | undefined;
  // The number shown while waiting for its computer to take it, and the wait itself.
  private _number = '';
  private _origin = '';
  private _name = '';
  private _waiting: AbortController | undefined;

  private readonly _handlers: DesktopAnswerHandlers = {
    computerRefused: (answer) => {
      this._adding = false;
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
      if (!this._adding) {
        return;
      }
      this._adding = false;
      this._refs.add.loading.set(false);
      this.say(ADDED, 'ok');
      this.clearForm();
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
    cancelWait: ref<JlButton>('.cancel-wait'),
    command: ref('.pair-command'),
    computers: ref('.computers'),
    copyCommand: ref<JlCopyButton>('.copy-command'),
    edit: ref(JlDialog),
    editCancel: ref<JlButton>('.edit-cancel'),
    editColor: ref<JlSelect>('.edit-color'),
    editName: ref<JlInput>('.edit-name'),
    editProblem: ref<JlAlert>('.edit-problem'),
    editSave: ref<JlButton>('.edit-save'),
    feedback: ref<JlAlert>('.feedback'),
    form: ref<HTMLFormElement>('form'),
    name: ref<JlInput>('#computer-name'),
    port: ref<JlInput>('#computer-port'),
    scan: ref<JlButton>('.scan'),
    scanner: ref(ScQrScanner),
    wait: ref(JlDialog),
    waiting: ref('.waiting'),
  }));

  templateHtml(): Node {
    return (
      <div class="computers-topic">
        <p class="lead">
          The window shows swcode on this computer, or on another one paired with this app once. Another computer’s
          swcode works on while this one is off, and its sessions notify you here.
        </p>
        <div class="computers" />
        <form>
          <h3>Add a computer</h3>
          <p class="lead">{CONNECT_LEAD}</p>
          <JlField label="Name" hint="What this app calls that computer.">
            <JlInput id="computer-name" autocomplete="off" />
          </JlField>
          <JlField class="address-field" label="Address">
            <JlInput id="computer-address" autocomplete="off" />
          </JlField>
          <JlField label="Port" hint="Empty for 4500 over HTTP, or the link’s own over HTTPS.">
            <JlInput id="computer-port" autocomplete="off" placeholder={DEFAULT_PORT} />
          </JlField>
          <JlAlert class="feedback" hidden />
          <div class="actions">
            <JlButton class="scan" icon="camera" label="Scan its address" hidden />
            <JlButton class="add" type="submit" variant="primary" label="Get a number" />
          </div>
        </form>
        <ScQrScanner />
        <JlDialog class="wait" heading="Pair a computer" size="sm">
          <div class="wait-body">
            <p class="lead">{WAIT_LEAD}</p>
            <div class="command">
              <code class="pair-command" />
              <JlCopyButton class="copy-command" />
            </div>
            <p class="waiting">{WAITING}</p>
          </div>
          <JlButton slot="actions" class="cancel-wait" label="Cancel" />
        </JlDialog>
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
    this._refs.scanner.events.register('found', (link) => this.scanned(link));
    this.on(this._refs.form, 'submit', (event) => {
      event.preventDefault();
      this.add();
    });
    this._refs.add.events.register('press', () => this.add());
    this._refs.cancelWait.events.register('press', () => this.cancel());
    this._refs.wait.events.register('close', () => this.cancel());
    this._refs.edit.events.register('close', () => this.stopEditing());
    this._refs.editCancel.events.register('press', () => this._refs.edit.close());
    this._refs.editSave.events.register('press', () => this.edit());
    onComputersAnswer((answer) => this.hear(answer), this.killSignal);
    askComputers({ type: 'listComputers' });
  }

  onDisconnect(): void {
    this.cancel();
  }

  /** What the form asks for where the computers are kept, and the scan button where the device can scan. */
  private renderForm(): void {
    const texts = ADDRESS_TEXTS[inDesktopApp() ? 'app' : 'browser'];
    this._refs.addressField.hint.set(texts.hint);
    this._refs.address.placeholder.set(texts.placeholder);
    this._refs.scan.hidden = inDesktopApp() || !canScan();
  }

  /** An address a QR code held goes in the form: the number is still asked of that computer. */
  private scanned(link: string): void {
    this._refs.address.value.set(link);
  }

  /** A number for the app to show, and the wait for the person to run `swcode pair` with it there. */
  private add(): void {
    if (this._waiting) {
      return;
    }
    const name = this._refs.name.value.get().trim();
    if (name === '') {
      this.say(NO_NAME, 'danger');
      return;
    }
    const target = pairingOrigin(this._refs.address.value.get(), this._refs.port.value.get(), location.protocol);
    if (!target.ok) {
      this.say(target.problem, 'danger');
      return;
    }
    this._name = name;
    this._origin = target.answer;
    this._number = newPairNumber();
    this.showFeedback('');
    this._refs.command.textContent = PAIR_COMMAND + this._number;
    this._refs.copyCommand.value.set(this._refs.command.textContent);
    this._refs.waiting.textContent = WAITING;
    this._refs.wait.heading.set(`Pair with ${name}`);
    this._refs.wait.show();
    const waiting = new AbortController();
    this._waiting = waiting;
    void this.waitFor(waiting);
  }

  private async waitFor(waiting: AbortController): Promise<void> {
    const answered = await waitForPairing(
      this._origin,
      this._number,
      deviceName(navigator.userAgent),
      waiting.signal
    );
    if (waiting.signal.aborted || !this.isConnected) {
      return;
    }
    this._waiting = undefined;
    this._refs.wait.close();
    if (!answered.ok) {
      this.say(answered.problem, 'danger');
      return;
    }
    this._adding = true;
    this._refs.add.loading.set(true);
    askComputers({ address: this._origin, name: this._name, token: answered.answer, type: 'addComputer' });
  }

  private cancel(): void {
    this._waiting?.abort();
    this._waiting = undefined;
  }

  private clearForm(): void {
    this._refs.address.value.set('');
    this._refs.name.value.set('');
  }

  private hear(answer: DesktopAnswer): void {
    this.handle(answer.type, answer);
  }

  private handle<Type extends keyof DesktopAnswerMap>(type: Type, answer: DesktopAnswerMap[Type]): void {
    this._handlers[type](answer);
  }

  private say(text: string, tone: Tone): void {
    this._refs.feedback.tone.set(tone);
    this.showFeedback(text);
  }

  private showFeedback(text: string): void {
    this._refs.feedback.description.set(text);
    this._refs.feedback.hidden = text === '';
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
