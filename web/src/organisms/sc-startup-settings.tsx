import { css } from '@jadis/core';
import { JlAlert, JlRadioGroup } from '@syware/jlib';

import { STARTUP_MODES } from '../../../shared/protocol/startup.schema';
import { ScElement } from '../base/sc-element';
import { loadStartup, saveStartup } from '../services/startup';
import { BASE_STYLES } from '../styles/shared';
import styles from './sc-startup-settings.css?inline';

import type { RadioOption, Tone } from '@syware/jlib';
import type { StartupMode, StartupView } from '../../../shared/protocol/startup.schema';

const SAVED = 'Saved: Windows starts swcode so from your next logon.';
const NOT_SAVED = 'Windows did not take it: nothing changed.';
const UNAVAILABLE = 'Only swcode’s Windows app starts with its computer: this swcode runs elsewhere.';

/** How each way to start reads in the list: a new way does not compile until it is here. */
const MODE_TEXTS: Readonly<Record<StartupMode, { name: string; hint: string }>> = {
  daemon: {
    hint: 'The daemon alone, without an icon: opening swcode shows it. For a computer others work on.',
    name: 'Without an icon',
  },
  off: { hint: 'You open swcode yourself, from the Start menu.', name: 'Don’t start' },
  tray: {
    hint: 'Its icon by the clock, the window closed: sessions work on and tell you when they need you.',
    name: 'In the notification area',
  },
};

const modeOf = (value: string): StartupMode | undefined => STARTUP_MODES.find((mode) => mode === value);

const MODE_OPTIONS: RadioOption<StartupMode>[] = STARTUP_MODES.map((mode) => ({
  description: MODE_TEXTS[mode].hint,
  label: MODE_TEXTS[mode].name,
  value: mode,
}));

/**
 * The settings' Startup: whether swcode starts when its person logs in to Windows, and how, as Windows starts other
 * programs. Each change applies at once.
 */
export class ScStartupSettings extends ScElement {
  static readonly selector = 'sc-startup-settings';

  private readonly _refs = this.useRefs((ref) => ({
    feedback: ref(JlAlert),
    modes: ref(JlRadioGroup),
  }));

  templateHtml(): Node {
    return (
      <div class="startup">
        <p class="lead">
          When you log in to Windows, swcode can start by itself, as Windows starts other programs: Windows’
          Settings › Apps › Startup lists it too.
        </p>
        <JlRadioGroup<StartupMode> label="When you log in" legend variant="cards" options={MODE_OPTIONS} />
        <JlAlert hidden />
      </div>
    );
  }

  templateCss(): string {
    return css`${BASE_STYLES}${styles}`;
  }

  onConnect(): void {
    this._refs.modes.events.register('change', (value) => void this.choose(modeOf(value)));
    void this.load();
  }

  private async load(): Promise<void> {
    const view = await loadStartup();
    if (!view || !this.isConnected) {
      return;
    }
    this.render(view);
  }

  private render(view: StartupView): void {
    this._refs.modes.disabled.set(!view.available);
    this._refs.modes.value.set(view.mode);
    if (!view.available) {
      this.say(UNAVAILABLE, 'info');
    }
  }

  private say(text: string, tone: Tone): void {
    this._refs.feedback.tone.set(tone);
    this._refs.feedback.description.set(text);
    this._refs.feedback.hidden = text === '';
  }

  private async choose(mode: StartupMode | undefined): Promise<void> {
    if (mode === undefined) {
      return;
    }
    this.say('', 'info');
    const saved = await saveStartup(mode);
    if (!this.isConnected) {
      return;
    }
    if (!saved) {
      this.say(NOT_SAVED, 'danger');
      await this.load();
      return;
    }
    this.render(saved);
    this.say(SAVED, 'ok');
  }
}

ScStartupSettings.register();
