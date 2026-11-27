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
  JlTabPanel,
  JlTabs,
} from '@syware/jlib';

import { COMPUTER_COLORS, ComputerColorSchema, THIS_COMPUTER } from '../../../shared/protocol/desktop.schema';
import { PAIR_NUMBER_DIGITS } from '../../../shared/protocol/pairing.schema';
import { ScElement } from '../base/sc-element';
import { newPairNumber } from '../lib/pair-number';
import {
  LOCAL_ONLY_HEADING,
  LOCAL_ONLY_NOTE,
  listensHereAlone,
  NO_ANSWER_HINT,
  nothingAnswersAt,
  PAIR_NUMBER_HINT,
  pairingWaitingText,
  reachableBaseOf,
} from '../lib/pairing-view';
import { PUSH_BUTTONS, PUSH_LINES, pushProblemShown } from '../lib/push-view';
import { qrElement } from '../lib/qr';
import { DEFAULT_PORT, pairingOrigin } from '../lib/remote-computers';
import { ScQrScanner } from '../molecules/sc-qr-scanner';
import { daemonOrigin } from '../services/access';
import { askComputers, onComputersAnswer } from '../services/computers';
import { daemonState, daemonStatusBus } from '../services/daemon-status';
import { inDesktopApp } from '../services/desktop';
import { installApp, installBus, installState } from '../services/install';
import { inAndroidApp } from '../services/native-app';
import { navigate, settingsPath } from '../services/navigation';
import {
  answersAt,
  deviceName,
  forgetDevice,
  pairedDevices,
  pendPairing,
  waitForPairing,
} from '../services/pairing';
import { pushState, turnPushOff, turnPushOn } from '../services/push';
import { canScan } from '../services/qr-scan';
import { BASE_STYLES } from '../styles/shared';
import styles from './sc-computers.css?inline';

import type { MenuItem, SelectOption, Tone } from '@syware/jlib';
import type { Computer, ComputerColor, DesktopAnswer } from '../../../shared/protocol/desktop.schema';
import type { Device } from '../../../shared/protocol/pairing.schema';
import type { ComputerAction, DesktopAnswerHandlers, DesktopAnswerMap } from '../types/desktop.types';
import type { InstallState } from '../types/install.types';
import type { PushState } from '../types/push.types';

// The settings topic where swcode is told which addresses to listen on.
const NETWORK_TOPIC = 'network';
const PAIR_COMMAND = 'swcode pair ';
const WAITING = 'Waiting for that computer to take the number…';
const WAIT_LEAD = 'On that computer, run this where its swcode runs. The number waits a few minutes.';
const ADDED = 'Added: open it to show its swcode here.';
const NO_NAME = 'Type the name this app should call that computer.';
const NO_DEVICES = 'No device is paired yet.';
const CONNECTED_LEAD =
  'The devices paired with this swcode hold a token of their own: a phone, or another computer. Each one’s Forget stops its token at once.';
const PAIR_LEAD = 'The device shows a number as it pairs: type it here, and it is paired with this swcode.';
const PAIR_NUMBER = new RegExp(`^\\d{${PAIR_NUMBER_DIGITS}}$`);
const QR_LABEL = 'QR code of the address swcode answers at on this network';
// What installing swcode on the device that shows this page takes, by where it stands.
const INSTALL_LINES: Readonly<Record<InstallState, string>> = {
  insecure:
    'A browser installs an app only from an HTTPS address, and this page came without it. Give swcode one (tailscale serve does, or a reverse proxy), then open swcode from that address.',
  installed: 'swcode is installed on this device: it opens from its own icon, in a window of its own.',
  manual:
    'In the browser’s menu, choose “Add to Home Screen” (in Safari, the Share button first) or “Install app”: swcode then opens from its own icon, in a window of its own.',
  offered: 'Install swcode to open it from its own icon, in a window of its own.',
};
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
// What the second tab is, by where the page stands: the Android app has no swcode of its own, only the one it paired
// with.
const CONNECT_PANEL_LEAD = {
  app: 'The window shows swcode on this computer, or on another one paired with this app once. Another computer’s swcode works on while this one is off, and its sessions notify you here.',
  phone:
    'The app shows swcode on the computer it is paired with, and on another one paired with it once. Another computer’s swcode works on while the first is off, and its sessions notify you here.',
} as const;
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
  // The computer the edit dialog is open on, until it closes, and whether its Save was asked.
  private _editing: Computer | undefined;
  private _saving = false;
  // Where notifications stood when last drawn: what a press on their button changes.
  private _pushState: PushState = 'unsupported';
  // Whether the address typed is being asked for an answer, before a number is shown for it.
  private _asking = false;
  // The number shown while waiting for its computer to take it, and the wait itself.
  private _number = '';
  private _origin = '';
  private _name = '';
  private _waiting: AbortController | undefined;

  private readonly _handlers: DesktopAnswerHandlers = {
    computerRefused: (answer) => {
      this._adding = false;
      this._saving = false;
      this._refs.add.loading.set(false);
      if (this._editing) {
        this.refuseEdit(answer.message);
        return;
      }
      this.say(answer.message, 'danger');
    },
    computers: (answer) => {
      this.renderComputers(answer.computers, answer.current);
      // The dialog closes on the answer to its own Save, not on a list that arrives while it is open.
      if (this._saving) {
        this._saving = false;
        this._refs.edit.close();
      }
      if (!this._adding) {
        return;
      }
      this._adding = false;
      this._refs.add.loading.set(false);
      this.say(ADDED, 'ok');
      this.clearForm();
      // Paired, it is this app's to name and colour: the dialog opens on it, rather than a form that asked before
      // the computer had even answered.
      const added = answer.computers.find((computer) => computer.address === this._origin);
      if (added) {
        this.openEdit(added);
      }
    },
    computerUnreachable: () => {
      // The switch says that computer did not answer; the list says which ones are not reachable.
      askComputers({ type: 'listComputers' });
    },
    // The app kept the answer: the section draws what it says, not what the press asked for. Asking again here
    // would be answered again, and so on without end.
    notifications: (answer) => this.drawPush(answer.on ? 'on' : 'off'),
  };

  private readonly _refs = this.useRefs((ref) => ({
    add: ref<JlButton>('.add'),
    address: ref<JlInput>('#computer-address'),
    addressField: ref<JlField>('.address-field'),
    cancelWait: ref<JlButton>('.cancel-wait'),
    command: ref('.pair-command'),
    computers: ref('.computers'),
    copyCommand: ref<JlCopyButton>('.copy-command'),
    devices: ref('.devices'),
    edit: ref<JlDialog>('.edit'),
    editCancel: ref<JlButton>('.edit-cancel'),
    editColor: ref<JlSelect>('.edit-color'),
    editName: ref<JlInput>('.edit-name'),
    editProblem: ref<JlAlert>('.edit-problem'),
    editSave: ref<JlButton>('.edit-save'),
    feedback: ref<JlAlert>('.feedback'),
    form: ref<HTMLFormElement>('form'),
    install: ref<JlButton>('.install-now'),
    installLine: ref('.install-line'),
    installSection: ref('.install'),
    link: ref('.link'),
    localOnly: ref<JlAlert>('.local-only'),
    name: ref<JlInput>('#computer-name'),
    notifications: ref('.notifications'),
    pairButton: ref<JlButton>('.pair-device'),
    pairInput: ref<JlInput>('.pair-input'),
    pairLine: ref<JlAlert>('.pair-line'),
    port: ref<JlInput>('#computer-port'),
    push: ref<JlButton>('.push'),
    pushLine: ref('.push-line'),
    pushProblem: ref('.push-problem'),
    qr: ref('.qr'),
    reach: ref('.reach'),
    scan: ref<JlButton>('.scan'),
    scanner: ref(ScQrScanner),
    toNetwork: ref<JlButton>('.to-network'),
    wait: ref<JlDialog>('.wait'),
    waiting: ref('.waiting'),
  }));

  templateHtml(): Node {
    return (
      <div class="computers-topic">
        <JlTabs label="Devices">
          <JlTabPanel name="connected" icon="laptop" label="Connected devices">
            <p class="lead">{CONNECTED_LEAD}</p>
            <JlAlert class="local-only" tone="warn" heading={LOCAL_ONLY_HEADING} hidden>
              {LOCAL_ONLY_NOTE}
              <JlButton slot="actions" class="to-network" label="Network settings" size="sm" />
            </JlAlert>
            <div class="pair">
              <h3>Pair a device</h3>
              <p class="lead">{PAIR_LEAD}</p>
              <JlField label="The number the device shows">
                <JlInput class="pair-input" autocomplete="off" placeholder="123456" />
              </JlField>
              <JlButton class="pair-device" variant="primary" label="Pair the device" />
              <JlAlert class="pair-line" hidden />
            </div>
            <div class="devices" />
            <section class="install" hidden>
              <h3>Install on this device</h3>
              <p class="lead install-line" />
              <JlButton class="install-now" icon="download" label="Install swcode" hidden />
            </section>
            <section class="notifications" hidden>
              <h3>Notifications on this device</h3>
              <p class="lead push-line" />
              <p class="note push-problem" hidden />
              <JlButton class="push" hidden />
            </section>
            <section class="reach">
              <h3>Open it on a phone</h3>
              <p class="lead">
                Open this address on the phone (its camera reads the code). Its page shows a number: type it here,
                and the phone is paired.
              </p>
              <div class="qr" />
              <p class="link" />
            </section>
          </JlTabPanel>
          <JlTabPanel name="connect" icon="plug" label="Connect a device">
            <p class="lead">{CONNECT_PANEL_LEAD[inAndroidApp() ? 'phone' : 'app']}</p>
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
          </JlTabPanel>
        </JlTabs>
        <ScQrScanner />
        <JlDialog class="wait" heading="Pair a computer" size="sm">
          <div class="wait-body">
            <p class="lead">{WAIT_LEAD}</p>
            <div class="command">
              <code class="pair-command" />
              <JlCopyButton class="copy-command" />
            </div>
            <p class="waiting">{WAITING}</p>
            <p class="lead">{NO_ANSWER_HINT}</p>
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
      void this.add();
    });
    this._refs.add.events.register('press', () => void this.add());
    this._refs.cancelWait.events.register('press', () => this.cancel());
    this._refs.wait.events.register('close', () => this.cancel());
    this._refs.edit.events.register('close', () => this.stopEditing());
    this._refs.editCancel.events.register('press', () => this._refs.edit.close());
    this._refs.editSave.events.register('press', () => this.edit());
    this._refs.pairButton.events.register('press', () => void this.pairDevice());
    this._refs.pairInput.events.register('enter', () => void this.pairDevice());
    this._refs.install.events.register('press', () => void installApp());
    this._refs.push.events.register('press', () => void this.togglePush());
    this._refs.toNetwork.events.register('press', () => navigate(settingsPath(NETWORK_TOPIC)));
    // The desktop app and the phone app are installed already: their window offers no install. Each notifies by its
    // own means (the tray through Windows, Android itself), which the section's switch turns on and off there.
    this._refs.installSection.hidden = inDesktopApp() || inAndroidApp();
    this._refs.notifications.hidden = false;
    // Installed, the app may notify where its page could not (an iPhone).
    this.onBus(installBus, 'changed', () => void this.renderPush());
    this.onBus(installBus, 'changed', () => this.renderInstall());
    this.onBus(daemonStatusBus, 'changed', () => this.renderPairing());
    this.renderInstall();
    this.renderPairing();
    void this.renderPush();
    onComputersAnswer((answer) => this.hear(answer), this.killSignal);
    askComputers({ type: 'listComputers' });
    void this.renderDevices();
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
  private async add(): Promise<void> {
    if (this._waiting || this._asking) {
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
    this.showFeedback('');
    // No number for an address nothing answers at: it would wait for nothing, and the person with it.
    this._asking = true;
    this._refs.add.loading.set(true);
    const answers = await answersAt(target.answer);
    this._asking = false;
    this._refs.add.loading.set(false);
    if (!this.isConnected) {
      return;
    }
    if (!answers) {
      this.say(nothingAnswersAt(target.answer), 'danger');
      return;
    }
    this._name = name;
    this._origin = target.answer;
    this._number = newPairNumber();
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

  /** Makes the number a device shows wait here for it to take: it pairs with this swcode, a phone or a computer. */
  private async pairDevice(): Promise<void> {
    const number = this._refs.pairInput.value.get().trim();
    if (!PAIR_NUMBER.test(number)) {
      this.sayPairing(PAIR_NUMBER_HINT, 'danger');
      return;
    }
    this._refs.pairButton.loading.set(true);
    const answer = await pendPairing(number);
    this._refs.pairButton.loading.set(false);
    if (!this.isConnected) {
      return;
    }
    this.sayPairing(
      answer.ok ? pairingWaitingText(number, answer.answer.lifetimeMs) : answer.problem,
      answer.ok ? 'ok' : 'danger'
    );
    if (!answer.ok) {
      return;
    }
    this._refs.pairInput.value.set('');
  }

  private sayPairing(text: string, tone: Tone): void {
    this._refs.pairLine.tone.set(tone);
    this._refs.pairLine.description.set(text);
    this._refs.pairLine.hidden = text === '';
  }

  /** The devices paired with this swcode, each holding a token of its own. */
  private async renderDevices(): Promise<void> {
    const devices = await pairedDevices();
    if (!this.isConnected) {
      return;
    }
    this._refs.devices.replaceChildren(
      ...(devices.length === 0
        ? [<p class="none">{NO_DEVICES}</p>]
        : devices.map((device) => this.deviceNode(device)))
    );
  }

  /** A paired device, and its forget button: a button keeps its clicks to itself, so each knows its device. */
  private deviceNode(device: Device): Node {
    const forget = createElement(JlButton, {
      attrs: { class: 'forget', slot: 'end' },
      props: {
        hint: `Forget ${device.name}: its token stops working`,
        label: 'Forget',
        size: 'sm',
        variant: 'ghost',
      },
    });
    forget.events.register('press', () => void forgetDevice(device.id).then(() => this.renderDevices()));
    const row = createElement(JlListItem, {
      attrs: { class: 'device' },
      props: {
        description: `paired ${new Date(device.pairedAt).toLocaleDateString()}`,
        heading: device.name,
        icon: 'smartphone',
      },
    });
    row.append(forget);
    return row;
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
    this._saving = true;
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

  /** How the device showing this page installs swcode, and the button that does it when the browser offers one. */
  private renderInstall(): void {
    const state = installState();
    this._refs.installLine.textContent = INSTALL_LINES[state];
    this._refs.install.hidden = state !== 'offered';
  }

  /** Where notifications stand on the device showing this page, and the button that changes it where one can. */
  private async renderPush(problem = ''): Promise<void> {
    const state = await pushState();
    if (this.isConnected) {
      this.drawPush(state, problem);
    }
  }

  private drawPush(state: PushState, problem = ''): void {
    this._pushState = state;
    const label = PUSH_BUTTONS[state];
    // A refusal that left notifications blocked is what the line itself says: it is said once.
    const said = pushProblemShown(state, problem);
    this._refs.pushLine.textContent = PUSH_LINES[state];
    this._refs.pushProblem.textContent = said;
    this._refs.pushProblem.hidden = said === '';
    this._refs.push.hidden = label === undefined;
    this._refs.push.label.set(label ?? '');
  }

  /** Turns notifications on for this device, or off once they are: what did not go through is said under it. */
  private async togglePush(): Promise<void> {
    this._refs.push.loading.set(true);
    const outcome = await (this._pushState === 'on' ? turnPushOff() : turnPushOn());
    this._refs.push.loading.set(false);
    await this.renderPush(outcome.ok ? '' : outcome.problem);
  }

  /** The address a phone opens: this computer's, as the daemon says it is reached. */
  private renderPairing(): void {
    const base = reachableBaseOf(daemonState(), daemonOrigin());
    // Listening on this computer alone, swcode has no address to give and no device to pair: saying so beats a code
    // that leads nowhere and a number that waits for nothing.
    const alone = listensHereAlone(daemonState(), daemonOrigin());
    this._refs.localOnly.hidden = !alone;
    this._refs.reach.hidden = alone;
    this._refs.pairInput.disabled.set(alone);
    this._refs.pairButton.disabled.set(alone);
    this._refs.qr.replaceChildren(qrElement(base, QR_LABEL));
    this._refs.link.textContent = base;
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
