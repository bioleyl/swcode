import { css } from '@jadis/core';
import { JlAlert, JlButton, JlDialog, JlField, JlInput, JlSelect } from '@syware/jlib';

import { ScElement } from '../base/sc-element';
import { newId } from '../lib/model-settings';
import { modelsAt } from '../services/settings';
import { BASE_STYLES } from '../styles/shared';
import styles from './sc-source-dialog.css?inline';

import type { SelectOption } from '@syware/jlib';
import type { LlmSource } from '../../../shared/protocol/models.schema';

const SECRET_TYPE = 'password';
const LOOKING = 'Looking for models…';
const NO_NAME = 'Name it: the presets show its name.';
const NO_ADDRESS = 'Type the address llama-server answers at.';
// jlib's select takes an empty value for nothing chosen: every slot, the default, has a value of its own here.
const ALL_SLOTS = 'all';

const SHARE_OPTIONS: SelectOption[] = [
  { label: 'All of them', value: ALL_SLOTS },
  { label: 'The first half (1/2)', value: '1/2' },
  { label: 'The second half (2/2)', value: '2/2' },
  { label: 'The first third (1/3)', value: '1/3' },
  { label: 'The second third (2/3)', value: '2/3' },
  { label: 'The last third (3/3)', value: '3/3' },
];

/**
 * Adds a llama-server or changes one: its name in the app, its address, its key, and its part of the slots when
 * people share it. The models it serves show under the address as it is typed. `save` gives the source; the editor
 * saves it, then closes the dialog or shows why not.
 */
export class ScSourceDialog extends ScElement {
  static readonly selector = 'sc-source-dialog';

  readonly events = this.useEvents<{ save: LlmSource }>();

  // The source changed, or a new one's id once named; the ids other sources have.
  private _id = '';
  private _taken: readonly string[] = [];
  // Each lookup's number: an answer that comes after a newer lookup started is not shown.
  private _lookups = 0;

  private readonly _refs = this.useRefs((ref) => ({
    address: ref<JlInput>('.address'),
    cancel: ref<JlButton>('.cancel'),
    dialog: ref(JlDialog),
    key: ref<JlInput>('.key'),
    name: ref<JlInput>('.name'),
    problem: ref(JlAlert),
    save: ref<JlButton>('.save'),
    served: ref('.served'),
    share: ref(JlSelect),
  }));

  templateHtml(): Node {
    return (
      <JlDialog heading="llama-server">
        <div class="body">
          <JlField label="Name" hint="How the presets and the side panel call it.">
            <JlInput class="name" autocomplete="off" placeholder="gmktec" />
          </JlField>
          <JlField label="Address" hint="Where llama-server answers.">
            <JlInput class="address" autocomplete="off" placeholder="http://127.0.0.1:8080" />
          </JlField>
          <p class="served" />
          <JlField label="API key" hint="Only when llama-server asks for one (its api-key option).">
            <JlInput class="key" autocomplete="off" placeholder="none" type={SECRET_TYPE} />
          </JlField>
          <JlField
            label="Slots this swcode uses"
            hint="When people share one llama-server, each with their own swcode: each takes a different part.">
            <JlSelect options={SHARE_OPTIONS} value={ALL_SLOTS} />
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
    this._refs.address.events.register('change', () => void this.lookUp());
    this._refs.key.events.register('change', () => void this.lookUp());
  }

  /** Opens on a source to change, or empty for a new one. */
  edit(source: LlmSource | undefined, taken: readonly string[]): void {
    this._id = source?.id ?? '';
    this._taken = taken;
    this._refs.dialog.heading.set(source ? `Change ${source.name}` : 'Add a llama-server');
    this._refs.name.value.set(source?.name ?? '');
    this._refs.address.value.set(source?.url ?? '');
    this._refs.key.value.set(source?.apiKey ?? '');
    this._refs.share.value.set(source?.slotShare ?? ALL_SLOTS);
    this._refs.save.loading.set(false);
    this.refuse('');
    this._refs.served.textContent = '';
    this._refs.dialog.show();
    void this.lookUp();
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
    const url = this._refs.address.value.get().trim();
    if (name === '') {
      this.refuse(NO_NAME);
      return;
    }
    if (url === '') {
      this.refuse(NO_ADDRESS);
      return;
    }
    const share = this._refs.share.value.get();
    const apiKey = this._refs.key.value.get().trim();
    this._refs.save.loading.set(true);
    this.events.emit('save', {
      id: this._id || newId(name, this._taken),
      name,
      url,
      ...(apiKey === '' ? {} : { apiKey }),
      ...(share === ALL_SLOTS || share === '' ? {} : { slotShare: share }),
    });
  }

  /** The models served at the address in the field, saved or not. */
  private async lookUp(): Promise<void> {
    const url = this._refs.address.value.get().trim();
    if (url === '') {
      this._refs.served.textContent = '';
      return;
    }
    this._lookups++;
    const lookup = this._lookups;
    this._refs.served.textContent = LOOKING;
    const answer = await modelsAt(url, this._refs.key.value.get().trim());
    if (lookup !== this._lookups || !this.isConnected) {
      return;
    }
    this._refs.served.textContent = answer.ok
      ? `${answer.models.length} served: ${answer.models.join(', ')}`
      : answer.problem;
  }
}

ScSourceDialog.register();
