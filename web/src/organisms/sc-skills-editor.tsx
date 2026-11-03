import { createElement, css } from '@jadis/core';
import { confirmDialog, JlAlert, JlButton, JlField, JlInput, JlListItem, JlTextarea } from '@syware/jlib';

import { ScElement } from '../base/sc-element';
import { createSkill, listSkills, loadSkill, removeSkill, updateSkill } from '../services/skills';
import { BASE_STYLES } from '../styles/shared';
import styles from './sc-skills-editor.css?inline';

import type { Tone } from '@syware/jlib';
import type { Skill, SkillSummary, SkillView } from '../../../shared/protocol/skill.schema';

const NEW_TITLE = 'New skill';
const SAVING = 'Saving…';
const SAVED = 'Saved. Sessions are told of it from their next turn on.';
const UNREACHABLE = 'The daemon did not answer: the skills could not be read.';
const NO_SKILLS = 'No skills yet: write one here, or ask the agent for one.';
const EMPTY_SKILL: SkillView = { description: '', files: [], instructions: '', name: '' };
// A description is one paragraph: the agent reads it in a line of the list of skills.
const DESCRIPTION_ROWS = 3;
const INSTRUCTIONS_ROWS = 14;
const LINE_BREAKS = /\s*\n\s*/g;
const INSTRUCTIONS_HINT = 'How to do the task, in markdown.';

const filesNote = (files: readonly string[]): string =>
  files.length === 0
    ? INSTRUCTIONS_HINT
    : `${INSTRUCTIONS_HINT} Also in its folder, for the agent to read: ${files.join(', ')}.`;

/**
 * swcode's own skills, in the settings: the user's instructions for kinds of tasks. The agent is told each skill's
 * name and description, and loads its instructions when a task fits. A skill opens in the editor under the list; a
 * new name renames it.
 */
export class ScSkillsEditor extends ScElement {
  static readonly selector = 'sc-skills-editor';

  // The skill in the editor, by the name it is saved under: undefined for a new one.
  private _editing: string | undefined;

  private readonly _refs = this.useRefs((ref) => ({
    create: ref<JlButton>('.create'),
    description: ref<JlTextarea>('#skill-description'),
    editor: ref<HTMLFormElement>('form'),
    empty: ref('.empty'),
    feedback: ref<JlAlert>('form jl-alert'),
    files: ref<JlField>('.instructions-field'),
    folder: ref('.folder'),
    instructions: ref<JlTextarea>('#skill-instructions'),
    list: ref('.list'),
    listProblem: ref<JlAlert>('.list-problem'),
    name: ref<JlInput>('#skill-name'),
    remove: ref<JlButton>('.remove'),
    save: ref<JlButton>('.save'),
    title: ref('.editor-title'),
  }));

  templateHtml(): Node {
    return (
      <div class="skills">
        <div class="head">
          <p class="lead">
            Instructions for kinds of tasks. The agent is told each skill’s name and description, and loads its
            instructions when a task fits. Ask it for one, or write one here. Kept in{' '}
            <code class="folder">~/.swcode/skills</code>; Claude Code’s come too, under Plugins.
          </p>
          <JlButton class="create" icon="plus" label="New skill" size="sm" />
        </div>
        <JlAlert class="list-problem" tone="danger" hidden />
        <div class="list" />
        <p class="empty" hidden />
        <form hidden>
          <h2 class="editor-title" />
          <JlField label="Name" hint="Lowercase letters, digits and hyphens. A new name renames the skill.">
            <JlInput id="skill-name" autocomplete="off" placeholder="deploy-staging" />
          </JlField>
          <JlField
            label="Description"
            hint="The agent picks a skill by this alone: say what it does and when to use it.">
            <JlTextarea
              id="skill-description"
              rows={DESCRIPTION_ROWS}
              placeholder="What it is for, and when to use it"
            />
          </JlField>
          <JlField class="instructions-field" label="Instructions" hint={INSTRUCTIONS_HINT}>
            <JlTextarea
              id="skill-instructions"
              rows={INSTRUCTIONS_ROWS}
              placeholder="How to do the task, step by step, in markdown"
            />
          </JlField>
          <JlAlert hidden />
          <div class="actions">
            <JlButton class="remove" icon="trash" label="Delete" variant="ghost" />
            <JlButton class="save" variant="primary" label="Save" />
          </div>
        </form>
      </div>
    );
  }

  templateCss(): string {
    return css`${BASE_STYLES}${styles}`;
  }

  onConnect(): void {
    this.on(this._refs.editor, 'submit', (event) => event.preventDefault());
    this._refs.create.events.register('press', () => this.startNew());
    this._refs.save.events.register('press', () => void this.save());
    this._refs.remove.events.register('press', () => void this.deleteSkill());
    void this.refresh();
  }

  private async refresh(): Promise<void> {
    const view = await listSkills();
    if (!this.isConnected) {
      return;
    }
    this._refs.listProblem.hidden = view !== undefined;
    this._refs.listProblem.description.set(UNREACHABLE);
    if (!view) {
      this.renderList([]);
      return;
    }
    this._refs.folder.textContent = view.folder;
    this.renderList(view.skills);
  }

  private renderList(skills: readonly SkillSummary[]): void {
    this._refs.empty.textContent = NO_SKILLS;
    this._refs.empty.hidden = skills.length > 0;
    this._refs.list.replaceChildren(...skills.map((skill) => this.itemNode(skill)));
  }

  /** A skill in the list: a row that opens it in the editor, marked while it is there. */
  private itemNode(skill: SkillSummary): Node {
    const item = createElement(JlListItem, {
      attrs: { class: 'item', 'data-name': skill.name },
      props: {
        description: skill.description,
        heading: skill.name,
        icon: 'book-open',
        interactive: true,
        selected: skill.name === this._editing,
      },
    });
    item.events.register('press', () => void this.open(skill.name));
    return item;
  }

  /** The skill in the editor stands out in the list. */
  private markCurrent(): void {
    for (const item of this._refs.list.querySelectorAll<JlListItem>('jl-list-item')) {
      item.selected.set(item.dataset.name === this._editing);
    }
  }

  private async open(name: string): Promise<void> {
    const skill = await loadSkill(name);
    if (!skill || !this.isConnected) {
      return;
    }
    this.edit(skill.name, skill);
    this._refs.editor.scrollIntoView({ behavior: 'smooth', block: 'start' });
  }

  private startNew(): void {
    this.edit(undefined, EMPTY_SKILL);
    this._refs.name.focus();
  }

  private edit(name: string | undefined, skill: SkillView): void {
    this._editing = name;
    this._refs.title.textContent = name ?? NEW_TITLE;
    this._refs.name.value.set(skill.name);
    this._refs.description.value.set(skill.description);
    this._refs.instructions.value.set(skill.instructions);
    this._refs.files.hint.set(filesNote(skill.files));
    this._refs.remove.hidden = name === undefined;
    this._refs.editor.hidden = false;
    this.say('', 'info');
    this.markCurrent();
  }

  private skillInEditor(): Skill {
    return {
      description: this._refs.description.value.get().replace(LINE_BREAKS, ' '),
      instructions: this._refs.instructions.value.get(),
      name: this._refs.name.value.get(),
    };
  }

  private async save(): Promise<void> {
    const skill = this.skillInEditor();
    this.say(SAVING, 'info');
    this._refs.save.loading.set(true);
    const editing = this._editing;
    const change = editing === undefined ? await createSkill(skill) : await updateSkill(editing, skill);
    this._refs.save.loading.set(false);
    if (!change.ok) {
      this.say(change.problem, 'danger');
      return;
    }
    this._editing = change.name;
    this._refs.title.textContent = change.name;
    this._refs.remove.hidden = false;
    this.say(SAVED, 'ok');
    await this.refresh();
  }

  private async deleteSkill(): Promise<void> {
    const name = this._editing;
    if (name === undefined) {
      return;
    }
    const confirmed = await confirmDialog({
      confirmLabel: 'Delete',
      heading: `Delete the skill ${name}?`,
      text: 'Its folder goes, with every file in it.',
      tone: 'danger',
    });
    if (!confirmed) {
      return;
    }
    const change = await removeSkill(name);
    if (!change.ok) {
      this.say(change.problem, 'danger');
      return;
    }
    this._editing = undefined;
    this._refs.editor.hidden = true;
    await this.refresh();
  }

  private say(text: string, tone: Tone): void {
    this._refs.feedback.tone.set(tone);
    this._refs.feedback.description.set(text);
    this._refs.feedback.hidden = text === '';
  }
}

ScSkillsEditor.register();
