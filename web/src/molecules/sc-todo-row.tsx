import { css } from '@jadis/core';
import { JlButton, JlCheckbox, JlInput, JlLink } from '@syware/jlib';

import { MAX_TODO_TEXT } from '../../../shared/protocol/project.schema';
import { ScElement } from '../base/sc-element';
import { BASE_STYLES } from '../styles/shared';
import styles from './sc-todo-row.css?inline';

import type { ProjectTodo } from '../../../shared/protocol/project.schema';

/**
 * One todo of a project: its text, a box that says it is done or that it is pinned to a new session, the buttons a
 * page offers (edit its words in place, delete it), and the session it started once one did.
 */
export class ScTodoRow extends ScElement {
  static readonly selector = 'sc-todo-row';

  readonly events = this.useEvents<{ change: boolean; remove: undefined; rename: string }>();

  readonly todo = this.useChange<ProjectTodo | undefined>(undefined, (todo) => this.drawTodo(todo));
  /** Whether the box is checked, as the page means it: done here, or pinned for a new session. */
  readonly checked = this.useChange(false, (checked) => this._refs.checkbox.checked.set(checked));
  readonly editable = this.useChange(false, (editable) => (this._refs.edit.hidden = !editable));
  readonly removable = this.useChange(false, (removable) => (this._refs.remove.hidden = !removable));
  /** The session the todo started, linked under its words: its address, and its title for the link. */
  readonly sessionHref = this.useChange('', () => this.drawSession());
  readonly sessionTitle = this.useChange('', () => this.drawSession());

  private readonly _refs = this.useRefs((ref) => ({
    cancel: ref<JlButton>('.cancel'),
    checkbox: ref(JlCheckbox),
    edit: ref<JlButton>('.edit'),
    editor: ref<HTMLFormElement>('.editor'),
    remove: ref<JlButton>('.remove'),
    session: ref(JlLink),
    text: ref(JlInput),
    view: ref('.view'),
  }));

  templateHtml(): Node {
    return (
      <div class="row">
        <div class="view">
          <JlCheckbox class="check" />
          <JlButton class="edit" icon="pencil" variant="ghost" size="sm" hint="Edit this todo" hidden />
          <JlButton class="remove" icon="trash" variant="ghost" size="sm" hint="Delete this todo" hidden />
        </div>
        <JlLink class="session" variant="quiet" hidden />
        <form class="editor" hidden>
          <JlInput class="text" autocomplete="off" maxlength={String(MAX_TODO_TEXT)} />
          <JlButton class="save" type="submit" variant="primary" size="sm" label="Save" />
          <JlButton class="cancel" variant="ghost" size="sm" label="Cancel" />
        </form>
      </div>
    );
  }

  templateCss(): string {
    return css`${BASE_STYLES}${styles}`;
  }

  onConnect(): void {
    this._refs.checkbox.events.register('change', (checked) => this.events.emit('change', checked));
    this._refs.edit.events.register('press', () => this.openEditor());
    this._refs.cancel.events.register('press', () => this.closeEditor());
    this._refs.remove.events.register('press', () => this.events.emit('remove'));
    this.on(this._refs.editor, 'submit', (event) => {
      event.preventDefault();
      this.save();
    });
  }

  private drawTodo(todo: ProjectTodo | undefined): void {
    this._refs.checkbox.label.set(todo?.text ?? '');
    this._refs.edit.hint.set(todo ? `Edit “${todo.text}”` : 'Edit this todo');
    this._refs.remove.hint.set(todo ? `Delete “${todo.text}”` : 'Delete this todo');
  }

  /** The link under the words names the session the todo started, or nothing when it started none. */
  private drawSession(): void {
    const href = this.sessionHref.get();
    const title = this.sessionTitle.get();
    const linked = href !== '' && title !== '';
    this._refs.session.hidden = !linked;
    this._refs.session.href.set(linked ? href : '');
    this._refs.session.label.set(linked ? `Pinned to “${title}”` : '');
  }

  /** The words become an editable copy, the box and the delete button leaving meanwhile. */
  private openEditor(): void {
    const todo = this.todo.get();
    if (!todo) {
      return;
    }
    this._refs.text.value.set(todo.text);
    this._refs.view.hidden = true;
    this._refs.editor.hidden = false;
    this._refs.text.focus();
  }

  private closeEditor(): void {
    this._refs.editor.hidden = true;
    this._refs.view.hidden = false;
  }

  private save(): void {
    const text = this._refs.text.value.get().trim();
    if (text !== '' && text !== this.todo.get()?.text) {
      this.events.emit('rename', text);
    }
    this.closeEditor();
  }
}

ScTodoRow.register();
