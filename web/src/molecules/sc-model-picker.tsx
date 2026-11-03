import { css } from '@jadis/core';
import { JlSelect } from '@syware/jlib';

import { ScElement } from '../base/sc-element';
import { modelName } from '../lib/model-name';
import { modelsAt } from '../services/settings';
import { BASE_STYLES } from '../styles/shared';
import styles from './sc-model-picker.css?inline';

import type { SelectOption } from '@syware/jlib';
import type { LlmSource, ModelRef } from '../../../shared/protocol/models.schema';

// jlib's select takes an empty value for nothing chosen: a model named by none has a value of its own here.
const NO_MODEL = 'default';

const toChoice = (model: string): string => (model === '' ? NO_MODEL : model);
const fromChoice = (value: string): string => (value === NO_MODEL ? '' : value);

/**
 * A model of a llama-server: the server among the sources, then a model it serves, listed once it answers. A model
 * it does not serve right now stays listed when it was chosen.
 */
export class ScModelPicker extends ScElement {
  static readonly selector = 'sc-model-picker';

  readonly sources = this.useChange<LlmSource[]>([], (sources) => {
    this._refs.source.options.set(sources.map((source) => ({ label: source.name, value: source.id })));
  });
  /** What naming no model means here: "Its first loaded model". */
  readonly empty = this.useChange('', () => this.renderModels([], this.chosenModel()));

  // Each lookup's number: an answer that comes after a newer lookup started is not shown.
  private _lookups = 0;

  private readonly _refs = this.useRefs((ref) => ({
    model: ref<JlSelect>('.model'),
    source: ref<JlSelect>('.source'),
  }));

  templateHtml(): Node {
    return (
      <div class="picker">
        <JlSelect class="source" label="llama-server" />
        <JlSelect class="model" label="Model" />
      </div>
    );
  }

  templateCss(): string {
    return css`${BASE_STYLES}${styles}`;
  }

  onConnect(): void {
    this._refs.source.events.register('change', () => void this.lookUp(''));
  }

  /** Shows a model, and looks up the others its server serves. */
  show(ref: ModelRef): void {
    this._refs.source.value.set(ref.source);
    void this.lookUp(ref.model);
  }

  /** The model chosen: none named when the server's own is. */
  value(): ModelRef {
    return { model: this.chosenModel(), source: this._refs.source.value.get() };
  }

  private chosenModel(): string {
    return fromChoice(this._refs.model.value.get());
  }

  private async lookUp(chosen: string): Promise<void> {
    this.renderModels([], chosen);
    const source = this.sources.get().find((each) => each.id === this._refs.source.value.get());
    if (!source) {
      return;
    }
    this._lookups++;
    const lookup = this._lookups;
    const answer = await modelsAt(source.url, source.apiKey ?? '');
    if (lookup !== this._lookups || !this.isConnected || !answer.ok) {
      return;
    }
    this.renderModels(answer.models, this.chosenModel());
  }

  private renderModels(served: readonly string[], chosen: string): void {
    const listed = chosen === '' || served.includes(chosen) ? served : [...served, chosen];
    const options: SelectOption[] = [
      { label: this.empty.get(), value: NO_MODEL },
      ...listed.map((model) => ({ label: modelName(model), value: model })),
    ];
    this._refs.model.options.set(options);
    this._refs.model.value.set(toChoice(chosen));
  }
}

ScModelPicker.register();
