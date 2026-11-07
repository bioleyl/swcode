import { css } from '@jadis/core';
import { JlButton, JlIcon, JlInput } from '@syware/jlib';

import { ScElement } from '../base/sc-element';
import { BASE_STYLES } from '../styles/shared';
import styles from './sc-password.css?inline';

/**
 * The password a command asked for: what it asked with, and a field to type it in, sent as Enter is pressed or the
 * button is pressed. Inline in the conversation on a computer, a sheet at the bottom of the screen on a phone. The
 * words typed never stay in the page once they are sent.
 */
export class ScPassword extends ScElement {
  static readonly selector = 'sc-password';

  readonly events = this.useEvents<{ answer: string; refuse: undefined }>();

  readonly prompt = this.useChange('', (value) => {
    this._refs.ask.textContent = value;
  });
  readonly where = this.useChange('', (value) => {
    this._refs.where.textContent = value;
  });

  private readonly _refs = this.useRefs((ref) => ({
    ask: ref('h2'),
    give: ref<JlButton>('.give'),
    refuse: ref<JlButton>('.refuse'),
    secret: ref<JlInput>('.secret'),
    where: ref('.where'),
  }));

  templateHtml(): Node {
    return (
      <section class="card" aria-label="A command is asking for a password">
        <span class="grabber" aria-hidden="true" />
        <div class="head">
          <span class="badge" aria-hidden="true">
            <JlIcon name="key-round" size={20} />
          </span>
          <div>
            <h2 />
            <p class="where" />
          </div>
        </div>
        <JlInput
          class="secret"
          autocomplete="off"
          label="The password the command asked for"
          placeholder="Your password"
          type="password"
        />
        <div class="answers">
          <JlButton class="refuse" label="No password" size="lg" />
          <JlButton class="give" label="Send" size="lg" variant="primary" />
        </div>
      </section>
    );
  }

  templateCss(): string {
    return css`${BASE_STYLES}${styles}`;
  }

  onConnect(): void {
    this._refs.secret.events.register('enter', () => this.send());
    this._refs.give.events.register('press', () => this.send());
    this._refs.refuse.events.register('press', () => this.events.emit('refuse', undefined));
  }

  /** What the person typed, and nothing left in the field: a password is not kept in the page. */
  private send(): void {
    const secret = this._refs.secret.value.get();
    this._refs.secret.value.set('');
    this.events.emit('answer', secret);
  }
}

ScPassword.register();
