import { css } from '@jadis/core';
import { JlButton, JlIcon } from '@syware/jlib';

import { ScElement } from '../base/sc-element';
import { sessionGrantLabel, toolLabel } from '../lib/tool-labels';
import { BASE_STYLES } from '../styles/shared';
import { ScDiff } from './sc-diff';
import styles from './sc-permission.css?inline';

import type { PermissionDecision, ToolDisplay, ToolName } from '../../../shared/protocol/session.schema';

const COMMAND_TOOL: ToolName = 'bash';

/**
 * A call that waits for a person: what it would do, exactly, and the answers. Inline in the conversation on a
 * computer, a sheet at the bottom of the screen on a phone.
 */
export class ScPermission extends ScElement {
  static readonly selector = 'sc-permission';

  readonly events = this.useEvents<{ answer: PermissionDecision }>();

  readonly tool = this.useChange<ToolName | undefined>(undefined, () => this.render());
  readonly summary = this.useChange('', () => this.render());
  readonly detail = this.useChange<ToolDisplay | undefined>(undefined, () => this.render());
  readonly where = this.useChange('', (value) => {
    this._refs.where.textContent = value;
  });

  private readonly _refs = this.useRefs((ref) => ({
    allow: ref<JlButton>('.allow'),
    always: ref<JlButton>('.always'),
    command: ref('pre'),
    deny: ref<JlButton>('.deny'),
    diff: ref<ScDiff>('sc-diff'),
    icon: ref<JlIcon>('.badge jl-icon'),
    path: ref('.path'),
    question: ref('h2'),
    where: ref('.where'),
    why: ref('.why'),
  }));

  templateHtml(): Node {
    return (
      <section class="card" aria-labelledby="question">
        <span class="grabber" aria-hidden="true" />
        <div class="head">
          <span class="badge" aria-hidden="true">
            <JlIcon size={20} />
          </span>
          <div>
            <h2 id="question" />
            <p class="where" />
          </div>
        </div>
        <pre hidden />
        <p class="path" hidden />
        <ScDiff hidden />
        <p class="why" hidden />
        <div class="answers">
          <JlButton class="deny" label="Deny" size="lg" />
          <JlButton class="allow" label="Allow once" variant="primary" size="lg" />
        </div>
        <JlButton class="always" variant="ghost" block hidden />
      </section>
    );
  }

  templateCss(): string {
    return css`${BASE_STYLES}${styles}`;
  }

  onConnect(): void {
    this._refs.deny.events.register('press', () => this.events.emit('answer', 'deny'));
    this._refs.allow.events.register('press', () => this.events.emit('answer', 'allowOnce'));
    this._refs.always.events.register('press', () => this.events.emit('answer', 'allowSession'));
  }

  private render(): void {
    const tool = this.tool.get();
    const summary = this.summary.get();
    const detail = this.detail.get();
    const label = toolLabel(tool);
    const isCommand = tool === COMMAND_TOOL;
    const grant = sessionGrantLabel(tool, summary);
    this._refs.icon.name.set(label.icon);
    this._refs.question.textContent = label.question;
    this._refs.command.hidden = !isCommand;
    this._refs.command.textContent = summary;
    this._refs.path.hidden = isCommand;
    this._refs.path.textContent = summary;
    this._refs.diff.hidden = detail?.kind !== 'diff';
    this._refs.diff.lines.set(detail?.kind === 'diff' ? detail.lines : []);
    this._refs.why.hidden = detail?.kind !== 'text';
    this._refs.why.textContent = detail?.kind === 'text' ? detail.text : '';
    this._refs.always.hidden = grant === undefined;
    this._refs.always.label.set(grant ?? '');
  }
}

ScPermission.register();
