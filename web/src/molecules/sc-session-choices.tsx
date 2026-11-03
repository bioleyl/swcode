import { css } from '@jadis/core';
import { JlField, JlLink, JlRadioGroup, JlSegmented, JlSwitch } from '@syware/jlib';

import { PERMISSION_MODES, THINKING_MODES } from '../../../shared/protocol/session.schema';
import { ScElement } from '../base/sc-element';
import { MODE_LABELS } from '../lib/mode-labels';
import { NO_PRESETS, presetDescription } from '../lib/presets';
import { hrefOf, settingsPath } from '../services/navigation';
import { BASE_STYLES } from '../styles/shared';
import styles from './sc-session-choices.css?inline';

import type { RadioOption, SegmentedOption } from '@syware/jlib';
import type { PresetChoices } from '../../../shared/protocol/models.schema';
import type { PermissionMode, ThinkingMode } from '../../../shared/protocol/session.schema';

const MODE_OPTIONS: RadioOption<PermissionMode>[] = PERMISSION_MODES.map((mode) => ({
  label: MODE_LABELS[mode],
  value: mode,
}));

// Short, to share a row: the group's title says what they are.
const THINKING_SHORT: Readonly<Record<ThinkingMode, string>> = {
  full: 'Full',
  low: 'Low',
  medium: 'Medium',
  off: 'Off',
};
const MODELS_TOPIC = 'models';
const THINKING_OPTIONS: SegmentedOption<ThinkingMode>[] = THINKING_MODES.map((thinking) => ({
  label: THINKING_SHORT[thinking],
  value: thinking,
}));

/**
 * What a session works with, all in view: its preset (the models, with what each preset runs on), what it asks before
 * doing, how much its model thinks, and token economy. Each pick says so at once.
 */
export class ScSessionChoices extends ScElement {
  static readonly selector = 'sc-session-choices';

  readonly events = this.useEvents<{
    preset: string;
    mode: PermissionMode;
    thinking: ThinkingMode;
    concise: boolean;
  }>();

  readonly choices = this.useChange<PresetChoices>(NO_PRESETS, (choices) => {
    this._refs.presets.options.set(
      choices.presets.map((preset) => ({
        description: presetDescription(choices, preset),
        label: preset.name,
        value: preset.id,
      }))
    );
    this._refs.presets.value.set(this.preset.get());
  });
  readonly preset = this.useChange('', (value) => this._refs.presets.value.set(value));
  readonly mode = this.useChange<PermissionMode>('auto', (value) => this._refs.mode.value.set(value));
  readonly thinking = this.useChange<ThinkingMode>('full', (value) => this._refs.thinking.value.set(value));
  readonly concise = this.useChange(false, (value) => this._refs.concise.checked.set(value));

  private readonly _refs = this.useRefs((ref) => ({
    concise: ref(JlSwitch),
    mode: ref<JlRadioGroup<PermissionMode>>('.mode'),
    presets: ref<JlRadioGroup>('.presets'),
    thinking: ref<JlSegmented<ThinkingMode>>('.thinking'),
  }));

  templateHtml(): Node {
    return (
      <div class="groups">
        <JlRadioGroup class="presets" label="Preset" legend />
        <JlLink class="edit" href={hrefOf(settingsPath(MODELS_TOPIC))} label="Edit the presets" />
        <JlRadioGroup<PermissionMode> class="mode" label="Permissions" legend options={MODE_OPTIONS} />
        <JlField label="Thinking">
          <JlSegmented<ThinkingMode>
            class="thinking"
            label="Thinking"
            block
            size="sm"
            options={THINKING_OPTIONS}
          />
        </JlField>
        <JlSwitch label="Token economy" />
      </div>
    );
  }

  templateCss(): string {
    return css`${BASE_STYLES}${styles}`;
  }

  onConnect(): void {
    this._refs.presets.events.register('change', (preset) => {
      this.preset.set(preset);
      this.events.emit('preset', preset);
    });
    this._refs.mode.events.register('change', (mode) => {
      this.mode.set(mode);
      this.events.emit('mode', mode);
    });
    this._refs.thinking.events.register('change', (thinking) => {
      this.thinking.set(thinking);
      this.events.emit('thinking', thinking);
    });
    this._refs.concise.events.register('change', (concise) => {
      this.concise.set(concise);
      this.events.emit('concise', concise);
    });
  }
}

ScSessionChoices.register();
