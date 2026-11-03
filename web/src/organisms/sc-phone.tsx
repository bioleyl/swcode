import { createElement, css } from '@jadis/core';
import { JlButton, JlDialog, JlListItem } from '@syware/jlib';

import { ScElement } from '../base/sc-element';
import { qrElement } from '../lib/qr';
import { forgetDevice, newPairingCode, pairedDevices, pairingLink } from '../services/pairing';
import { BASE_STYLES } from '../styles/shared';
import styles from './sc-phone.css?inline';

import type { Device, PairingCode } from '../../../shared/protocol/pairing.schema';

const LOCAL_HOSTS = new Set(['localhost', '127.0.0.1', '[::1]']);
const LOCAL_NOTE =
  'This address only works on this computer: the daemon listens here alone. Start it with SWCODE_HOST=0.0.0.0 for the network, or SWCODE_PUBLIC_URL set to an address the phone reaches.';
const QR_LABEL = 'QR code of the link that pairs a phone with swcode';
const MS_PER_SECOND = 1000;
// A new code comes this long before the shown one expires, so the QR code never shows a dead one.
const RENEW_AHEAD_MS = 2000;
const NO_CODE = 'The daemon gives no pairing code: is it running?';
const NO_DEVICES = 'No phone is paired yet.';

const isLocal = (base: string): boolean => LOCAL_HOSTS.has(new URL(base).hostname);

/**
 * How to use swcode from a phone: a QR code with a pairing code that serves once and changes every minute (a phone
 * trades it for a token of its own), and the phones paired so far.
 */
export class ScPhone extends ScElement {
  static readonly selector = 'sc-phone';

  /** The address a phone opens: the daemon's public one, or this page's. */
  readonly base = this.useChange('', () => this.renderPairing());

  private _code: PairingCode | undefined;
  private _renewing = false;
  private _clock: ReturnType<typeof setInterval> | undefined;

  private readonly _refs = this.useRefs((ref) => ({
    close: ref<JlButton>('.close'),
    devices: ref('.devices'),
    dialog: ref(JlDialog),
    expires: ref('.expires'),
    link: ref('.link'),
    note: ref('.note'),
    qr: ref('.qr'),
  }));

  templateHtml(): Node {
    return (
      <JlDialog heading="Use swcode on a phone">
        <section>
          <h3>Pair</h3>
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
          <h3>Paired phones</h3>
          <div class="devices" />
        </section>
        <JlButton slot="actions" class="close" label="Done" variant="primary" />
      </JlDialog>
    );
  }

  templateCss(): string {
    return css`${BASE_STYLES}${styles}`;
  }

  onConnect(): void {
    this._refs.close.events.register('press', () => this._refs.dialog.close());
    this._refs.dialog.events.register('close', () => this.stopClock());
  }

  onDisconnect(): void {
    this.stopClock();
  }

  open(): void {
    this._code = undefined;
    this.renderPairing();
    void this.renew();
    this.startClock();
    this._refs.dialog.show();
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
    const left = this._code ? Date.parse(this._code.expiresAt) - Date.now() : 0;
    if (left <= RENEW_AHEAD_MS) {
      void this.renew();
    }
    this.renderExpiry(left);
  }

  private async renew(): Promise<void> {
    if (this._renewing) {
      return;
    }
    this._renewing = true;
    this._code = await newPairingCode();
    this._renewing = false;
    this.renderPairing();
  }

  private renderPairing(): void {
    const base = this.base.get();
    if (base === '') {
      return;
    }
    const code = this._code;
    const link = code ? pairingLink(base, code.code) : '';
    this._refs.qr.replaceChildren(...(code ? [qrElement(link, QR_LABEL)] : []));
    this._refs.link.textContent = link;
    this.renderExpiry(code ? Date.parse(code.expiresAt) - Date.now() : 0);
    this._refs.note.hidden = !isLocal(base);
    this._refs.note.textContent = LOCAL_NOTE;
  }

  private renderExpiry(left: number): void {
    this._refs.expires.textContent = this._code
      ? `A new code in ${Math.max(0, Math.ceil((left - RENEW_AHEAD_MS) / MS_PER_SECOND))} s`
      : NO_CODE;
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
