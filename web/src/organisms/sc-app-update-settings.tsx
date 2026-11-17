import { css } from '@jadis/core';
import { JlAlert, JlButton } from '@syware/jlib';

import { ScElement } from '../base/sc-element';
import { appUpdateNote } from '../lib/app-update';
import { checkAppUpdate } from '../services/app-update';
import { BASE_STYLES } from '../styles/shared';
import styles from './sc-app-update-settings.css?inline';

/**
 * The settings' Updates in the Android app: the release the app takes from GitHub, and a look for it now. The
 * daemon's own updates are a computer's, which the app does not install.
 */
export class ScAppUpdateSettings extends ScElement {
  static readonly selector = 'sc-app-update-settings';

  private readonly _refs = this.useRefs((ref) => ({
    line: ref(JlAlert),
    look: ref<JlButton>('.look'),
  }));

  templateHtml(): Node {
    return (
      <div class="app-update">
        <p class="lead">
          The Android app takes its next release from GitHub. It looks when it comes back; this looks now.
        </p>
        <JlButton class="look" icon="refresh" label="Look now" />
        <JlAlert hidden />
      </div>
    );
  }

  templateCss(): string {
    return css`${BASE_STYLES}${styles}`;
  }

  onConnect(): void {
    this._refs.look.events.register('press', () => void this.lookForUpdate());
  }

  /** Asks the app to look for a newer release now, and says what it found under the button. */
  private async lookForUpdate(): Promise<void> {
    this._refs.look.loading.set(true);
    const update = await checkAppUpdate();
    this._refs.look.loading.set(false);
    if (!this.isConnected) {
      return;
    }
    const note = appUpdateNote(update);
    this._refs.line.tone.set(note.tone);
    this._refs.line.description.set(note.text);
    this._refs.line.hidden = false;
  }
}

ScAppUpdateSettings.register();
