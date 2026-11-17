import { createElement, css } from '@jadis/core';
import { confirmDialog, JlAlert, JlBadge, JlButton, JlListItem } from '@syware/jlib';

import { ScElement } from '../base/sc-element';
import {
  presetUsing,
  sourceDescription,
  withoutPreset,
  withoutSource,
  withPreset,
  withSource,
} from '../lib/model-settings';
import { presetDescription } from '../lib/presets';
import { loadModels, saveModels } from '../services/models';
import { BASE_STYLES } from '../styles/shared';
import styles from './sc-models-editor.css?inline';
import { ScPresetDialog } from './sc-preset-dialog';
import { ScSourceDialog } from './sc-source-dialog';

import type { Tone } from '@syware/jlib';
import type { LlmSource, ModelSettings, Preset, PresetChoices } from '../../../shared/protocol/models.schema';
import type { ModelEditorKind } from '../types/settings.types';

const UNREADABLE = 'The daemon did not say its models.';
const SAVED = 'Saved: sessions use them from their next request.';
const SOURCES_KIND: ModelEditorKind = 'sources';
const PRESETS_KIND: ModelEditorKind = 'presets';
const SOURCES_LEAD =
  'The llm-servers the agents ask. Kept in ~/.swcode/models.json, in use as soon as they change: a session at work goes on with the models it started with.';
const PRESETS_LEAD =
  'The presets each session picks in its composer: a model of these servers for the main agent and the subagents. Kept in ~/.swcode/models.json, in use as soon as they change.';

const choicesOf = (settings: ModelSettings): PresetChoices => ({
  defaultPreset: settings.defaultPreset,
  presets: settings.presets,
  sources: settings.sources.map((source) => ({ id: source.id, name: source.name })),
});

/**
 * The settings' Models: the llm-servers the agents ask, or the presets each session picks from them in its composer,
 * one of the two by its `kind`. Each change saves them all, and the daemon uses them at once.
 */
export class ScModelsEditor extends ScElement {
  static readonly selector = 'sc-models-editor';

  /** Which list this editor shows, set once by the page that makes it: the llm-servers, or the presets. */
  readonly kind = this.useChange<ModelEditorKind>(SOURCES_KIND, () => undefined);

  /** Whether its tab is shown: the models are read then, as another tab may have changed them. */
  readonly active = this.useChange(false, (active) => (active ? void this.load() : undefined));

  private _settings: ModelSettings | undefined;

  private readonly _refs = this.useRefs((ref) => ({
    addPreset: ref<JlButton>('.add-preset'),
    addSource: ref<JlButton>('.add-source'),
    feedback: ref(JlAlert),
    presetDialog: ref(ScPresetDialog),
    presets: ref('.presets'),
    sourceDialog: ref(ScSourceDialog),
    sources: ref('.sources'),
  }));

  private get kindName(): ModelEditorKind {
    return this.kind.get();
  }

  templateHtml(): Node {
    return (
      <div class="models-topic">
        <p class="lead">{this.kindName === PRESETS_KIND ? PRESETS_LEAD : SOURCES_LEAD}</p>
        <JlAlert hidden />
        {this.kindName === PRESETS_KIND ? this.presetsNode() : this.sourcesNode()}
      </div>
    );
  }

  templateCss(): string {
    return css`${BASE_STYLES}${styles}`;
  }

  onConnect(): void {
    const register = this.kindName === PRESETS_KIND ? () => this.registerPresets() : () => this.registerSources();
    register();
    if (this.active.get()) {
      void this.load();
    }
  }

  private registerSources(): void {
    this._refs.addSource.events.register('press', () => this.editSource(undefined));
    this._refs.sourceDialog.events.register('save', (source) => void this.saveSource(source));
  }

  private registerPresets(): void {
    this._refs.addPreset.events.register('press', () => this.editPreset(undefined));
    this._refs.presetDialog.events.register('save', (preset) => void this.savePreset(preset));
  }

  private sourcesNode(): Node {
    return (
      <section>
        <div class="actions">
          <JlButton class="add-source" icon="plus" label="Add" size="sm" />
        </div>
        <div class="sources" />
        <ScSourceDialog />
      </section>
    );
  }

  private presetsNode(): Node {
    return (
      <section>
        <div class="actions">
          <JlButton class="add-preset" icon="plus" label="Add" size="sm" />
        </div>
        <div class="presets" />
        <ScPresetDialog />
      </section>
    );
  }

  private async load(): Promise<void> {
    const settings = await loadModels();
    if (!this.isConnected) {
      return;
    }
    if (!settings) {
      this.say(UNREADABLE, 'danger');
      return;
    }
    this.render(settings);
  }

  private say(text: string, tone: Tone): void {
    this._refs.feedback.tone.set(tone);
    this._refs.feedback.description.set(text);
    this._refs.feedback.hidden = text === '';
  }

  /** Saves the models as changed: the problem when the daemon refuses them, nothing once they are in use. */
  private async save(settings: ModelSettings): Promise<string> {
    const saved = await saveModels(settings);
    if (!saved.ok) {
      return saved.problem;
    }
    this.render(settings);
    this.say(SAVED, 'ok');
    return '';
  }

  /** Saves the models as changed, from a row's button: what goes wrong shows over the lists. */
  private async apply(settings: ModelSettings): Promise<void> {
    const problem = await this.save(settings);
    if (problem === '') {
      return;
    }
    this.say(problem, 'danger');
  }

  private editSource(source: LlmSource | undefined): void {
    const settings = this._settings;
    if (!settings) {
      return;
    }
    this._refs.sourceDialog.edit(
      source,
      settings.sources.map((each) => each.id)
    );
  }

  private editPreset(preset: Preset | undefined): void {
    const settings = this._settings;
    if (!settings) {
      return;
    }
    this._refs.presetDialog.edit(preset, settings);
  }

  private async saveSource(source: LlmSource): Promise<void> {
    const settings = this._settings;
    if (!settings) {
      return;
    }
    const problem = await this.save(withSource(settings, source));
    if (problem !== '') {
      this._refs.sourceDialog.refuse(problem);
      return;
    }
    this._refs.sourceDialog.close();
  }

  private async savePreset(preset: Preset): Promise<void> {
    const settings = this._settings;
    if (!settings) {
      return;
    }
    const problem = await this.save(withPreset(settings, preset));
    if (problem !== '') {
      this._refs.presetDialog.refuse(problem);
      return;
    }
    this._refs.presetDialog.close();
  }

  /** A source goes once no preset uses it, after a confirmation. */
  private async removeSource(source: LlmSource): Promise<void> {
    const settings = this._settings;
    if (!settings) {
      return;
    }
    const user = presetUsing(settings, source.id);
    if (user) {
      this.say(`The preset ${user.name} uses ${source.name}: change it or remove it first.`, 'danger');
      return;
    }
    const confirmed = await confirmDialog({
      confirmLabel: 'Remove',
      heading: `Remove ${source.name}?`,
      text: 'swcode stops asking it; llm-server itself goes on.',
      tone: 'danger',
    });
    if (!confirmed) {
      return;
    }
    await this.apply(withoutSource(settings, source.id));
  }

  /** A preset goes after a confirmation; its sessions take the default one. */
  private async removePreset(preset: Preset): Promise<void> {
    const settings = this._settings;
    if (!settings) {
      return;
    }
    const confirmed = await confirmDialog({
      confirmLabel: 'Remove',
      heading: `Remove the preset ${preset.name}?`,
      text: 'The sessions working with it take the default preset.',
      tone: 'danger',
    });
    if (!confirmed) {
      return;
    }
    await this.apply(withoutPreset(settings, preset.id));
  }

  private async makeDefault(preset: Preset): Promise<void> {
    const settings = this._settings;
    if (!settings) {
      return;
    }
    await this.apply({ ...settings, defaultPreset: preset.id });
  }

  private render(settings: ModelSettings): void {
    this._settings = settings;
    if (this.kindName === PRESETS_KIND) {
      const choices = choicesOf(settings);
      this._refs.presets.replaceChildren(
        ...settings.presets.map((preset) => this.presetNode(preset, choices, preset.id === settings.defaultPreset))
      );
      return;
    }
    this._refs.sources.replaceChildren(...settings.sources.map((source) => this.sourceNode(source)));
  }

  /** A button keeps its clicks to itself: each row's own know their source. */
  private sourceNode(source: LlmSource): Node {
    const row = createElement(JlListItem, {
      props: { description: sourceDescription(source), heading: source.name, icon: 'server' },
    });
    row.append(
      this.rowButton('Change', `Change ${source.name}`, () => this.editSource(source)),
      this.rowButton('Remove', `Remove ${source.name}`, () => void this.removeSource(source))
    );
    return row;
  }

  private presetNode(preset: Preset, choices: PresetChoices, isDefault: boolean): Node {
    const row = createElement(JlListItem, {
      props: { description: presetDescription(choices, preset), heading: preset.name, icon: 'sliders-horizontal' },
    });
    if (isDefault) {
      row.append(createElement(JlBadge, { attrs: { slot: 'end' }, props: { label: 'Default', tone: 'accent' } }));
    }
    if (!isDefault) {
      row.append(
        this.rowButton('Make default', `New sessions take ${preset.name}`, () => void this.makeDefault(preset))
      );
    }
    row.append(this.rowButton('Change', `Change ${preset.name}`, () => this.editPreset(preset)));
    // The default preset stays: another one becomes the default first.
    if (!isDefault) {
      row.append(this.rowButton('Remove', `Remove ${preset.name}`, () => void this.removePreset(preset)));
    }
    return row;
  }

  private rowButton(label: string, hint: string, press: () => void): JlButton {
    const button = createElement(JlButton, {
      attrs: { slot: 'end' },
      props: { hint, label, size: 'sm', variant: 'ghost' },
    });
    button.events.register('press', press);
    return button;
  }
}

ScModelsEditor.register();
