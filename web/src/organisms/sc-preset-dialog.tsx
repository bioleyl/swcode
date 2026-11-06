import { css } from '@jadis/core';
import { JlAlert, JlButton, JlDialog, JlField, JlInput, JlSelect } from '@syware/jlib';

import { ScElement } from '../base/sc-element';
import { newId } from '../lib/model-settings';
import {
  contextText,
  readTokens,
  SUBAGENT_LIMIT_OPTIONS,
  subagentLimitChoice,
  subagentLimitOf,
} from '../lib/presets';
import { ScModelPicker } from '../molecules/sc-model-picker';
import { BASE_STYLES } from '../styles/shared';
import styles from './sc-preset-dialog.css?inline';

import type { ModelRef, ModelSettings, Preset } from '../../../shared/protocol/models.schema';
import type { TokensInput } from '../types/presets.types';

const NO_NAME = 'Name it: the composer shows its name.';
const MAIN_EMPTY = 'Its model, or a router’s first loaded';
const SUBAGENTS_EMPTY = 'The main agent’s, on the same llm-server';
const CONTEXT_EMPTY = 'As the llm-server says';
const CONTEXT_HINT =
  'Tokens a conversation holds, compacted before it is full: 131072, or 128k. The llm-server’s own, when it says one, is the most.';
const BAD_CONTEXT = 'A context is a number of tokens: 131072, or 128k.';

const withContext = (ref: ModelRef, typed: TokensInput): ModelRef =>
  typed.kind === 'tokens' ? { ...ref, contextTokens: typed.tokens } : ref;

/**
 * Adds a preset or changes one: its name, the main agent's model, the subagents' (which reads web pages too), each a
 * model of one of the llm-servers, and how many subagents a turn may start. `save` gives the preset; the editor
 * saves it, then closes the dialog or shows why not.
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
    mainContext: ref<JlInput>('.main-context'),
    name: ref<JlInput>('.name'),
    problem: ref(JlAlert),
    save: ref<JlButton>('.save'),
    subagentLimit: ref<JlSelect>('.subagent-limit'),
    subagents: ref<ScModelPicker>('.subagents'),
    subagentsContext: ref<JlInput>('.subagents-context'),
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
          <JlField label="Main agent’s context" hint={CONTEXT_HINT}>
            <JlInput class="main-context" autocomplete="off" placeholder={CONTEXT_EMPTY} />
          </JlField>
          <JlField
            label="Subagents"
            hint="They search and read for the main agent, and read web pages: a smaller, faster model reads sooner, on another computer too.">
            <ScModelPicker class="subagents" empty={SUBAGENTS_EMPTY} />
          </JlField>
          <JlField label="Subagents’ context" hint={CONTEXT_HINT}>
            <JlInput class="subagents-context" autocomplete="off" placeholder={CONTEXT_EMPTY} />
          </JlField>
          <JlField
            label="Subagents at once"
            hint="How many work at the same time; the others wait their turn. Keep it below the slots of their llm-server (--parallel) when the main agent uses it too, so that it answers meanwhile. With none, the main agent is not told they exist.">
            <JlSelect class="subagent-limit" options={[...SUBAGENT_LIMIT_OPTIONS]} />
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

  /** Opens on a preset to change, or on a new one, of the first llm-server. */
  edit(preset: Preset | undefined, settings: ModelSettings): void {
    const first = settings.sources[0]?.id ?? '';
    this._id = preset?.id ?? '';
    this._taken = settings.presets.map((each) => each.id);
    this._refs.dialog.heading.set(preset ? `Change ${preset.name}` : 'Add a preset');
    this._refs.name.value.set(preset?.name ?? '');
    for (const picker of [this._refs.main, this._refs.subagents]) {
      picker.sources.set(settings.sources);
    }
    this._refs.main.show(preset?.main ?? { model: '', source: first });
    this._refs.subagents.show(preset?.workers ?? { model: '', source: first });
    this._refs.mainContext.value.set(this.contextShown(preset?.main));
    this._refs.subagentsContext.value.set(this.contextShown(preset?.workers));
    this.showLimit(subagentLimitChoice(preset?.subagentLimit));
    this.refuse('');
    this._refs.dialog.show();
  }

  private contextShown(ref: ModelRef | undefined): string {
    return ref?.contextTokens === undefined ? '' : contextText(ref.contextTokens);
  }

  /** The preset's limit, listed among the choices even when it is none of them (set in models.json, say). */
  private showLimit(choice: string): void {
    const select = this._refs.subagentLimit;
    const listed = SUBAGENT_LIMIT_OPTIONS.some((option) => option.value === choice);
    select.options.set(
      listed ? [...SUBAGENT_LIMIT_OPTIONS] : [...SUBAGENT_LIMIT_OPTIONS, { label: choice, value: choice }]
    );
    select.value.set(choice);
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
    const mainContext = readTokens(this._refs.mainContext.value.get());
    const subagentsContext = readTokens(this._refs.subagentsContext.value.get());
    if (mainContext.kind === 'invalid' || subagentsContext.kind === 'invalid') {
      this.refuse(BAD_CONTEXT);
      return;
    }
    this._refs.save.loading.set(true);
    const limit = subagentLimitOf(this._refs.subagentLimit.value.get());
    this.events.emit('save', {
      id: this._id || newId(name, this._taken),
      main: withContext(this._refs.main.value(), mainContext),
      name,
      workers: withContext(this._refs.subagents.value(), subagentsContext),
      ...(limit === undefined ? {} : { subagentLimit: limit }),
    });
  }
}

ScPresetDialog.register();
