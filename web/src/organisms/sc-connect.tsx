import { css } from '@jadis/core';
import { JlAlert, JlButton, JlCopyButton, JlField, JlInput } from '@syware/jlib';

import { ScBrand } from '../atoms/sc-brand';
import { ScElement } from '../base/sc-element';
import { newPairNumber } from '../lib/pair-number';
import { DEFAULT_PORT, pairingOrigin } from '../lib/remote-computers';
import { keepToken } from '../services/access';
import { connectTo } from '../services/browser-computers';
import { daemonState } from '../services/daemon-status';
import { deviceName, waitForPairing } from '../services/pairing';
import { BASE_STYLES } from '../styles/shared';
import styles from './sc-connect.css?inline';

const PAIR_COMMAND = 'swcode pair ';
const SERVED_LEAD = 'This computer’s swcode does not know this device yet. Run this on the computer:';
const FLEET_LEAD = 'Add the computer whose swcode you want to use.';
const WAITING = 'Waiting for that computer to take the number…';
const NO_NAME = 'Type the name this app should call that computer.';

/**
 * What a page shows when it can talk to no swcode yet: a page a daemon serves without knowing the device, or the app
 * a phone installed before it was given a computer. It shows a number, which a person types on the computer with
 * `swcode pair`, and waits for that computer to take it.
 */
export class ScConnect extends ScElement {
  static readonly selector = 'sc-connect';

  /** Whether it is on show: the number is drawn then, for a page its daemon does not know. */
  readonly active = this.useChange(false, (active) => (active ? this.start() : this.stop()));

  private _origin = '';
  private _name = '';
  private _number = '';
  private _waiting: AbortController | undefined;

  private readonly _refs = this.useRefs((ref) => ({
    address: ref<JlInput>('#connect-address'),
    command: ref('.connect-command'),
    copyCommand: ref<JlCopyButton>('.connect-copy'),
    form: ref<HTMLFormElement>('form'),
    name: ref<JlInput>('#connect-name'),
    port: ref<JlInput>('#connect-port'),
    problem: ref<JlAlert>('.connect-problem'),
    served: ref('.served'),
    submit: ref<JlButton>('.connect-submit'),
    waiting: ref('.waiting'),
  }));

  templateHtml(): Node {
    return (
      <div class="connect">
        <ScBrand />
        <h1>Connect to swcode</h1>
        <p class="lead">{FLEET_LEAD}</p>
        <form>
          <JlField label="Name" hint="What this app calls that computer.">
            <JlInput id="connect-name" autocomplete="off" />
          </JlField>
          <JlField
            label="Address"
            hint="Its name or IP address, or a whole link; HTTPS when this app came over it.">
            <JlInput id="connect-address" autocomplete="off" />
          </JlField>
          <JlField label="Port" hint="Empty for 4500 over HTTP, or the link’s own over HTTPS.">
            <JlInput id="connect-port" autocomplete="off" placeholder={DEFAULT_PORT} />
          </JlField>
          <JlButton class="connect-submit" type="submit" variant="primary" label="Get a number" />
        </form>
        <section class="served">
          <p class="lead">{SERVED_LEAD}</p>
          <div class="number">
            <code class="connect-command" />
            <JlCopyButton class="connect-copy" />
          </div>
          <p class="waiting">{WAITING}</p>
        </section>
        <JlAlert class="connect-problem" tone="danger" hidden />
      </div>
    );
  }

  templateCss(): string {
    return css`${BASE_STYLES}${styles}`;
  }

  onConnect(): void {
    this.on(this._refs.form, 'submit', (event) => {
      event.preventDefault();
      this.add();
    });
    this._refs.submit.events.register('press', () => this.add());
    if (this.active.get()) {
      this.start();
    }
  }

  onDisconnect(): void {
    this.stop();
  }

  /** A daemon serves this page but refuses its token: the address is that daemon's, and it needs no typing. */
  private served(): boolean {
    return daemonState().kind === 'locked';
  }

  private start(): void {
    this._refs.form.hidden = this.served();
    this._refs.served.hidden = !this.served();
    this.refuse('');
    // The shell sets `active` at every status answer, and a set fires even with the value it had: a number already
    // waiting stays, or it would change on the phone every fifteen seconds.
    if (this.served() && this._waiting === undefined) {
      this.begin(location.origin, '');
    }
  }

  private stop(): void {
    this._waiting?.abort();
    this._waiting = undefined;
  }

  /** The number the person types on the computer, and the wait for that computer to take it. */
  private begin(origin: string, name: string): void {
    this._origin = origin;
    this._name = name;
    this._number = newPairNumber();
    this._refs.command.textContent = PAIR_COMMAND + this._number;
    this._refs.copyCommand.value.set(this._refs.command.textContent);
    this._refs.waiting.textContent = WAITING;
    this._refs.served.hidden = false;
    const waiting = new AbortController();
    this._waiting = waiting;
    void this.waitFor(waiting);
  }

  private add(): void {
    if (this._waiting) {
      return;
    }
    const name = this._refs.name.value.get().trim();
    if (name === '') {
      this.refuse(NO_NAME);
      return;
    }
    const target = pairingOrigin(this._refs.address.value.get(), this._refs.port.value.get(), location.protocol);
    if (!target.ok) {
      this.refuse(target.problem);
      return;
    }
    this.refuse('');
    this._refs.form.hidden = true;
    this.begin(target.answer, name);
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
    if (!answered.ok) {
      this.failed(answered.problem);
      return;
    }
    if (this.served()) {
      keepToken(answered.answer);
      location.reload();
      return;
    }
    connectTo(this._origin, this._name, answered.answer);
  }

  /** The wait that got nowhere: the form comes back, with what went wrong. */
  private failed(problem: string): void {
    this._refs.form.hidden = this.served();
    this._refs.served.hidden = !this.served();
    this.refuse(problem);
  }

  private refuse(problem: string): void {
    this._refs.problem.description.set(problem);
    this._refs.problem.hidden = problem === '';
  }
}

ScConnect.register();
