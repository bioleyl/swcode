import { createElement, css } from '@jadis/core';
import { JlButton, JlListItem } from '@syware/jlib';

import { ScElement } from '../base/sc-element';
import { isLocalBase, LOCAL_ADDRESS_NOTE, reachableBaseOf, tailnetLine } from '../lib/pairing-view';
import { PUSH_BUTTONS, PUSH_LINES, pushProblemShown } from '../lib/push-view';
import { qrElement } from '../lib/qr';
import { daemonOrigin } from '../services/access';
import { daemonState, daemonStatusBus, refreshStatus } from '../services/daemon-status';
import { inDesktopApp } from '../services/desktop';
import { installApp, installBus, installState } from '../services/install';
import { inAndroidApp } from '../services/native-app';
import { forgetDevice, pairedDevices } from '../services/pairing';
import { pushState, turnPushOff, turnPushOn } from '../services/push';
import { readTailnet, serveOnTailnet } from '../services/tailnet';
import { BASE_STYLES } from '../styles/shared';
import styles from './sc-phone.css?inline';

import type { Device, Tailnet } from '../../../shared/protocol/pairing.schema';
import type { InstallState } from '../types/install.types';
import type { PushState } from '../types/push.types';

const QR_LABEL = 'QR code of the address swcode answers at on this network';
const NO_DEVICES = 'No phone is paired yet.';
// What installing swcode on the device that shows this page takes, by where it stands.
const INSTALL_LINES: Readonly<Record<InstallState, string>> = {
  insecure:
    'A browser installs an app only from an HTTPS address, and this page came without it. Give swcode one (a tailnet does, above), then open swcode from that address.',
  installed: 'swcode is installed on this device: it opens from its own icon, in a window of its own.',
  manual:
    'In the browser’s menu, choose “Add to Home Screen” (in Safari, the Share button first) or “Install app”: swcode then opens from its own icon, in a window of its own.',
  offered: 'Install swcode to open it from its own icon, in a window of its own.',
};

/**
 * The settings' Phone topic, how to use swcode from a phone: the address it answers at, as a QR code a phone opens,
 * whose page then shows a number to type here with `swcode pair`, and the phones paired so far.
 */
export class ScPhone extends ScElement {
  static readonly selector = 'sc-phone';

  /** Whether its topic is shown: the address is drawn then, and the paired phones read. */
  readonly active = this.useChange(false, (active) => (active ? this.start() : undefined));

  // Where notifications stood when last drawn: what a press on their button changes.
  private _pushState: PushState = 'unsupported';

  private readonly _refs = this.useRefs((ref) => ({
    devices: ref('.devices'),
    install: ref<JlButton>('.install-now'),
    installLine: ref('.install-line'),
    installSection: ref('.install'),
    link: ref('.link'),
    note: ref('.note'),
    notifications: ref('.notifications'),
    push: ref<JlButton>('.push'),
    pushLine: ref('.push-line'),
    pushProblem: ref('.push-problem'),
    qr: ref('.qr'),
    serve: ref<JlButton>('.serve'),
    tailnet: ref('.tailnet'),
    tailnetLine: ref('.tailnet-line'),
    tailnetProblem: ref('.tailnet-problem'),
  }));

  templateHtml(): Node {
    return (
      <div class="phone">
        <section>
          <h2>Pair a phone</h2>
          <p class="lead">
            Open this address on the phone (its camera reads the code). The page it shows gives a number: run
            swcode pair with that number here, and the phone is paired.
          </p>
          <div class="qr" />
          <p class="link" />
          <p class="note" hidden />
        </section>
        <section>
          <h2>Paired phones</h2>
          <div class="devices" />
        </section>
        <section class="tailnet" hidden>
          <h2>Over HTTPS, on your tailnet</h2>
          <p class="lead tailnet-line" />
          <p class="note tailnet-problem" hidden />
          <JlButton class="serve" label="Serve swcode on the tailnet" hidden />
        </section>
        <section class="install">
          <h2>Install on this device</h2>
          <p class="lead install-line" />
          <JlButton class="install-now" icon="download" label="Install swcode" hidden />
        </section>
        <section class="notifications">
          <h2>Notifications on this device</h2>
          <p class="lead push-line" />
          <p class="note push-problem" hidden />
          <JlButton class="push" hidden />
        </section>
      </div>
    );
  }

  templateCss(): string {
    return css`${BASE_STYLES}${styles}`;
  }

  onConnect(): void {
    // The desktop app is installed already, and tells the person of a session that waits through Windows itself:
    // its window offers neither an install nor web notifications, which it refuses. The phone app is installed too,
    // and its WebView has no push service.
    this._refs.installSection.hidden = inDesktopApp() || inAndroidApp();
    this._refs.notifications.hidden = inDesktopApp() || inAndroidApp();
    this._refs.install.events.register('press', () => void installApp());
    this._refs.serve.events.register('press', () => void this.serve());
    this._refs.push.events.register('press', () => void this.togglePush());
    // Installed, the app may notify where its page could not (an iPhone).
    this.onBus(installBus, 'changed', () => void this.renderPush());
    void this.renderPush();
    this.onBus(installBus, 'changed', () => this.renderInstall());
    // The address a phone reaches may change: the tailnet's, once Tailscale serves swcode there.
    this.onBus(daemonStatusBus, 'changed', () => this.renderPairing());
    this.renderInstall();
    this.renderPairing();
    if (this.active.get()) {
      this.start();
    }
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
    if (!this.isConnected) {
      return;
    }
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

  /** The address, the phones paired so far, and what the tailnet can do. */
  private start(): void {
    this.renderPairing();
    void this.renderDevices();
    void readTailnet().then((tailnet) => this.renderTailnet(tailnet));
  }

  /** Has Tailscale serve swcode: the QR code then leads to its HTTPS address, once the daemon's status says it. */
  private async serve(): Promise<void> {
    this._refs.serve.loading.set(true);
    const tailnet = await serveOnTailnet();
    this._refs.serve.loading.set(false);
    this.renderTailnet(tailnet);
    await refreshStatus();
  }

  /** The tailnet's part: nothing for a daemon that does not answer, its address or why none, and the way to it. */
  private renderTailnet(tailnet: Tailnet | undefined): void {
    this._refs.tailnet.hidden = tailnet === undefined;
    if (tailnet === undefined) {
      return;
    }
    this._refs.tailnetLine.textContent = tailnetLine(tailnet);
    // Available and still with a problem: Tailscale refused to serve, and said why.
    const refused = tailnet.available && tailnet.problem !== undefined;
    this._refs.tailnetProblem.hidden = !refused;
    this._refs.tailnetProblem.textContent = refused ? (tailnet.problem ?? '') : '';
    this._refs.serve.hidden = !tailnet.available || tailnet.served;
  }

  private renderPairing(): void {
    const base = reachableBaseOf(daemonState(), daemonOrigin());
    this._refs.qr.replaceChildren(qrElement(base, QR_LABEL));
    this._refs.link.textContent = base;
    this._refs.note.hidden = !isLocalBase(base);
    this._refs.note.textContent = LOCAL_ADDRESS_NOTE;
  }

  private async renderDevices(): Promise<void> {
    const devices = await pairedDevices();
    this._refs.devices.replaceChildren(
      ...(devices.length === 0
        ? [<p class="none">{NO_DEVICES}</p>]
        : devices.map((device) => this.deviceNode(device)))
    );
  }

  /** A paired phone, and its forget button: a button keeps its clicks to itself, so each knows its phone. */
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
}

ScPhone.register();
