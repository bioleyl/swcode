import { css } from '@jadis/core';
import { JlIcon } from '@syware/jlib';

import { ScElement } from '../base/sc-element';
import { BASE_STYLES } from '../styles/shared';
import styles from './sc-session-todos.css?inline';

import type { ProjectTodo } from '../../../shared/protocol/project.schema';

const TODO_ICON_SIZE = 12;

/** The project's todos this session started, on one line above its composer: which ones, and which are done. */
export class ScSessionTodos extends ScElement {
  static readonly selector = 'sc-session-todos';

  readonly todos = this.useChange<ProjectTodo[]>([], (todos) => this.render(todos));

  private readonly _refs = this.useRefs((ref) => ({
    list: ref('.list'),
  }));

  templateHtml(): Node {
    return (
      <section class="linked">
        <span class="title">Linked todos</span>
        <span class="list" />
      </section>
    );
  }

  templateCss(): string {
    return css`${BASE_STYLES}${styles}`;
  }

  private render(todos: ProjectTodo[]): void {
    this.hidden = todos.length === 0;
    this._refs.list.replaceChildren(...todos.map((todo) => this.rowOf(todo)));
  }

  private rowOf(todo: ProjectTodo): Node {
    return (
      <span class={todo.done ? 'todo done' : 'todo'}>
        <JlIcon name={todo.done ? 'check' : 'circle'} size={TODO_ICON_SIZE} />
        <span class="text">{todo.text}</span>
      </span>
    );
  }
}

ScSessionTodos.register();
