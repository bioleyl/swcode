import { css } from '@jadis/core';
import { JlAlert, JlButton, JlDialog, JlField, JlInput } from '@syware/jlib';

import { ScElement } from '../base/sc-element';
import { newId } from '../lib/model-settings';
import { ScModelPicker } from '../molecules/sc-model-picker';
import { BASE_STYLES } from '../styles/shared';
import styles from './sc-preset-dialog.css?inline';

import type { ModelSettings, Preset } from '../../../shared/protocol/models.schema';

const NO_NAME = 'Name it: the composer shows its name.';
const MAIN_EMPTY = 'Its model, or a router’s first loaded';
const WORKERS_EMPTY = 'The main agent’s, on the same llama-server';

/**
 * Adds a preset or changes one: its name, the main agent's model, and the workers' (subagents, the web page reader),
 * each a model of one of the llama-servers. `save` gives the preset; the editor saves it, then closes the dialog or
 * shows why not.
 */
export class ScPresetDialog extends ScElement {
  static readonly selector = 'sc-preset-dialog';

  readonly events = this.useEvents<{ save: Preset }>();

  // The preset changed, or a new one's id once named; the ids other presets have.
  private _id = '';
  private _taken: readonly string[] = [];

  private readonly _refs = this.useRefs((ref) => ({
    cancel: ref<JlButton>('.cancel'),
    dialog: ref(JlDialog),
    main: ref<ScModelPicker>('.main'),
    name: ref<JlInput>('.name'),
    problem: ref(JlAlert),
    save: ref<JlButton>('.save'),
    workers: ref<ScModelPicker>('.workers'),
  }));

  templateHtml(): Node {
    return (
      <JlDialog heading="Preset" size="lg">
        <div class="body">
          <JlField label="Name" hint="What the composer’s chip says.">
            <JlInput class="name" autocomplete="off" placeholder="Big and fast" />
          </JlField>
          <JlField label="Main agent" hint="It plans, edits and answers: the best model there is.">
            <ScModelPicker class="main" empty={MAIN_EMPTY} />
          </JlField>
          <JlField
            label="Workers"
            hint="Subagents and the web page reader: a smaller, faster model reads sooner, on another computer too.">
            <ScModelPicker class="workers" empty={WORKERS_EMPTY} />
          </JlField>
          <JlAlert tone="danger" hidden />
        </div>
        <JlButton slot="actions" class="cancel" label="Cancel" />
        <JlButton slot="actions" class="save" variant="primary" label="Save" />
      </JlDialog>
    );
  }

  templateCss(): string {
    return css`${BASE_STYLES}${styles}`;
  }

  onConnect(): void {
    this._refs.cancel.events.register('press', () => this.close());
    this._refs.save.events.register('press', () => this.submit());
  }

  /** Opens on a preset to change, or on a new one, of the first llama-server. */
  edit(preset: Preset | undefined, settings: ModelSettings): void {
    const first = settings.sources[0]?.id ?? '';
    this._id = preset?.id ?? '';
    this._taken = settings.presets.map((each) => each.id);
    this._refs.dialog.heading.set(preset ? `Change ${preset.name}` : 'Add a preset');
    this._refs.name.value.set(preset?.name ?? '');
    for (const picker of [this._refs.main, this._refs.workers]) {
      picker.sources.set(settings.sources);
    }
    this._refs.main.show(preset?.main ?? { model: '', source: first });
    this._refs.workers.show(preset?.workers ?? { model: '', source: first });
    this.refuse('');
    this._refs.dialog.show();
  }

  /** Why the editor could not save it: the dialog stays open on it. */
  refuse(problem: string): void {
    this._refs.save.loading.set(false);
    this._refs.problem.description.set(problem);
    this._refs.problem.hidden = problem === '';
  }

  close(): void {
    this._refs.dialog.close();
  }

  private submit(): void {
    const name = this._refs.name.value.get().trim();
    if (name === '') {
      this.refuse(NO_NAME);
      return;
    }
    this._refs.save.loading.set(true);
    this.events.emit('save', {
      id: this._id || newId(name, this._taken),
      main: this._refs.main.value(),
      name,
      workers: this._refs.workers.value(),
    });
  }
}

ScPresetDialog.register();
