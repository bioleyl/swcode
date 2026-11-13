import { createElement, css } from '@jadis/core';
import { JlButton, JlListItem } from '@syware/jlib';

import { ScElement } from '../base/sc-element';
import {
  isLocalBase,
  LOCAL_ADDRESS_NOTE,
  reachableBaseOf,
  renewalLine,
  secondsUntilRenewal,
} from '../lib/pairing-view';
import { qrElement } from '../lib/qr';
import { daemonOrigin } from '../services/access';
import { daemonState } from '../services/daemon-status';
import { installApp, installBus, installState } from '../services/install';
import { forgetDevice, NO_CODE, newPairingCode, pairedDevices, pairingLink } from '../services/pairing';
import { BASE_STYLES } from '../styles/shared';
import styles from './sc-phone.css?inline';

import type { Device, PairingCode } from '../../../shared/protocol/pairing.schema';
import type { InstallState } from '../types/install.types';

const QR_LABEL = 'QR code of the link that pairs a phone with swcode';
const MS_PER_SECOND = 1000;
const NO_DEVICES = 'No phone is paired yet.';
// What installing swcode on the device that shows this page takes, by where it stands.
const INSTALL_LINES: Readonly<Record<InstallState, string>> = {
  insecure:
    'A browser installs an app only from an HTTPS address, and this page came without it. Reach this computer over HTTPS (a tailnet gives it one: tailscale serve), then pair from that address.',
  installed: 'swcode is installed on this device: it opens from its own icon, in a window of its own.',
  manual:
    'In the browser’s menu, choose “Add to Home Screen” (in Safari, the Share button first) or “Install app”: swcode then opens from its own icon, in a window of its own.',
  offered: 'Install swcode to open it from its own icon, in a window of its own.',
};

/**
 * The settings' Phone topic, how to use swcode from a phone: a QR code with a pairing code that serves once and
 * changes every minute (a phone trades it for a token of its own), and the phones paired so far. It makes codes only
 * while its topic is shown.
 */
export class ScPhone extends ScElement {
  static readonly selector = 'sc-phone';

  /** Whether its topic is shown. */
  readonly active = this.useChange(false, (active) => (active ? this.start() : this.stopClock()));

  private _code: PairingCode | undefined;
  // Why there is no code: what the daemon said, or that it does not answer.
  private _problem = NO_CODE;
  private _renewing = false;
  private _clock: ReturnType<typeof setInterval> | undefined;

  private readonly _refs = this.useRefs((ref) => ({
    devices: ref('.devices'),
    expires: ref('.expires'),
    install: ref<JlButton>('.install-now'),
    installLine: ref('.install-line'),
    link: ref('.link'),
    note: ref('.note'),
    qr: ref('.qr'),
  }));

  templateHtml(): Node {
    return (
      <div class="phone">
        <section>
          <h2>Pair a phone</h2>
          <p class="lead">
            Scan this with the phone's camera, on the same network as this computer. The code works once and
            changes every minute: the phone that uses it gets a token of its own.
          </p>
          <div class="qr" />
          <p class="expires" />
          <p class="link" />
          <p class="note" hidden />
        </section>
        <section>
          <h2>Paired phones</h2>
          <div class="devices" />
        </section>
        <section>
          <h2>Install on this device</h2>
          <p class="lead install-line" />
          <JlButton class="install-now" icon="download" label="Install swcode" hidden />
        </section>
      </div>
    );
  }

  templateCss(): string {
    return css`${BASE_STYLES}${styles}`;
  }

  onConnect(): void {
    this._refs.install.events.register('press', () => void installApp());
    this.onBus(installBus, 'changed', () => this.renderInstall());
    this.renderInstall();
    if (this.active.get() && this._clock === undefined) {
      this.start();
    }
  }

  /** How the device showing this page installs swcode, and the button that does it when the browser offers one. */
  private renderInstall(): void {
    const state = installState();
    this._refs.installLine.textContent = INSTALL_LINES[state];
    this._refs.install.hidden = state !== 'offered';
  }

  onDisconnect(): void {
    this.stopClock();
  }

  /** A first code, renewed every minute, and the phones paired so far. */
  private start(): void {
    this._code = undefined;
    this.renderPairing();
    void this.renew();
    this.startClock();
    void this.renderDevices();
  }

  /** Every second while the dialog is open: the time left, and a new code before this one expires. */
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
    const code = this._code;
    const link = code ? pairingLink(base, code.code) : '';
    this._refs.qr.replaceChildren(...(code ? [qrElement(link, QR_LABEL)] : []));
    this._refs.link.textContent = link;
    this.renderExpiry();
    this._refs.note.hidden = !isLocalBase(base);
    this._refs.note.textContent = LOCAL_ADDRESS_NOTE;
  }

  /** The seconds left, or why there is no code to wait for. */
  private renderExpiry(): void {
    const code = this._code;
    this._refs.expires.textContent = code
      ? renewalLine(secondsUntilRenewal(code.expiresAt, Date.now()))
      : this._problem;
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
