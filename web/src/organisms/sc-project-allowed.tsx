import { createElement, css } from '@jadis/core';
import { JlButton, JlCard, JlListItem } from '@syware/jlib';

import { ScElement } from '../base/sc-element';
import { ALLOWED_RULE_LABELS, allowedRows } from '../lib/project-allowed';
import { removeProjectAllowed } from '../services/project-list';
import { BASE_STYLES } from '../styles/shared';
import styles from './sc-project-allowed.css?inline';

import type { Project } from '../../../shared/protocol/project.schema';
import type { AllowedRow } from '../types/project-allowed.types';

const NOTE =
  'What every session of this project does without asking, whatever its mode. A rule is added when you answer a request with “Always allow … in this project”.';
const NONE = 'Nothing yet.';

/**
 * A project's rules, in its Config tab: the programs and MCP servers its sessions use without asking, each taken
 * back with its button; sessions ask about it again from their next call.
 */
export class ScProjectAllowed extends ScElement {
  static readonly selector = 'sc-project-allowed';

  readonly project = this.useChange<Project | undefined>(undefined, () => this.render());

  // What the rows show: the list is drawn again only when it changes, so a button keeps its focus.
  private _shown = '';

  private readonly _refs = this.useRefs((ref) => ({
    none: ref('.none'),
    rows: ref('.rows'),
  }));

  templateHtml(): Node {
    return (
      <JlCard class="allowed" heading="Allowed without asking" description={NOTE}>
        <div class="rows" />
        <p class="none">{NONE}</p>
      </JlCard>
    );
  }

  templateCss(): string {
    return css`${BASE_STYLES}${styles}`;
  }

  onConnect(): void {
    this.render();
  }

  private render(): void {
    const project = this.project.get();
    if (!project || !this.isConnected) {
      return;
    }
    const rows = allowedRows(project);
    const shown = `${project.id}\n${rows.map((row) => `${row.rule} ${row.name}`).join('\n')}`;
    if (shown === this._shown) {
      return;
    }
    this._shown = shown;
    this._refs.rows.replaceChildren(...rows.map((row) => this.rowNode(project.id, row)));
    this._refs.none.hidden = rows.length > 0;
  }

  /** A rule and its button: a button keeps its clicks to itself, so each knows its rule. */
  private rowNode(projectId: string, row: AllowedRow): Node {
    const label = ALLOWED_RULE_LABELS[row.rule];
    const remove = createElement(JlButton, {
      attrs: { class: 'remove', slot: 'end' },
      props: { hint: `Ask about ${row.name} again`, label: 'Remove', size: 'sm', variant: 'ghost' },
    });
    remove.events.register('press', () => removeProjectAllowed(projectId, row.rule, row.name));
    const item = createElement(JlListItem, {
      attrs: { class: 'rule' },
      props: { description: label.says, heading: row.name, icon: label.icon },
    });
    item.append(remove);
    return item;
  }
}

ScProjectAllowed.register();
