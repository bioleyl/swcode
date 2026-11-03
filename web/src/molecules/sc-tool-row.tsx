import { css } from '@jadis/core';
import { JlIcon } from '@syware/jlib';

import { ScElement } from '../base/sc-element';
import { toolLabel } from '../lib/tool-labels';
import { toolMeta } from '../lib/tool-meta';
import { authorizedUrl } from '../services/access';
import { BASE_STYLES } from '../styles/shared';
import { ScDiff } from './sc-diff';
import { ScPlanSteps } from './sc-plan-steps';
import styles from './sc-tool-row.css?inline';

import type { ToolDisplay, ToolName } from '../../../shared/protocol/session.schema';
import type { SubagentLine, ToolResult, ToolRowState } from '../types/conversation.types';

const OPEN = 'open';
const CALL_ICON_SIZE = 14;

const callNode = (line: SubagentLine): Node => {
  const label = toolLabel(line.tool);
  return (
    <div class="call" data-ok={String(line.ok)}>
      <JlIcon name={label.icon} size={CALL_ICON_SIZE} />
      <span class="call-name">{label.name}</span>
      <span class="call-summary">{line.summary}</span>
      <span class="call-meta">{line.meta ?? ''}</span>
    </div>
  );
};

const STATE_ICONS: Readonly<Record<ToolRowState, string>> = {
  done: 'check',
  failed: 'x',
  pending: '',
  running: '',
  waiting: '',
};

/** One tool call in the conversation: what it does in a line, and what it changed or printed when opened. */
export class ScToolRow extends ScElement {
  static readonly selector = 'sc-tool-row';

  readonly tool = this.useChange<ToolName | undefined>(undefined, (tool) => {
    const label = toolLabel(tool);
    this._refs.icon.name.set(label.icon);
    this._refs.name.textContent = label.name;
    this.reflect('tool', tool ?? false);
  });
  readonly summary = this.useChange('', (value) => {
    this._refs.summary.textContent = value;
    this._refs.summary.title = value;
  });
  readonly state = this.useChange<ToolRowState>('pending', () => this.renderState());
  readonly result = this.useChange<ToolResult | undefined>(undefined, (result) => {
    this.renderState();
    this.renderDisplay(result?.display);
  });
  /** The calls of the subagent an agent call started, as they finish. */
  readonly activity = this.useChange<SubagentLine[]>([], (lines) => this.renderActivity(lines));

  private readonly _refs = this.useRefs((ref) => ({
    calls: ref('.calls'),
    details: ref('.details'),
    diff: ref<ScDiff>('sc-diff'),
    head: ref<HTMLButtonElement>('.head'),
    icon: ref<JlIcon>('.icon'),
    image: ref('img'),
    meta: ref('.meta-text'),
    name: ref('.name'),
    output: ref('pre'),
    picture: ref('a'),
    plan: ref<ScPlanSteps>('sc-plan-steps'),
    stateIcon: ref<JlIcon>('.state-icon'),
    summary: ref('.summary'),
  }));

  templateHtml(): Node {
    return (
      <div class="row">
        <button class="head" type="button" aria-expanded="false" disabled>
          <JlIcon class="icon" />
          <span class="name" />
          <span class="summary" />
          <span class="meta">
            <span class="dot" aria-hidden="true" />
            <JlIcon class="state-icon" size={14} />
            <span class="meta-text" />
          </span>
          <JlIcon class="chevron" name="chevron-down" size={14} />
        </button>
        <div class="details" hidden>
          <div class="calls" hidden />
          <ScDiff hidden />
          <ScPlanSteps hidden />
          <pre hidden />
          <a class="picture" target="_blank" rel="noopener" hidden>
            <img alt="" />
          </a>
        </div>
      </div>
    );
  }

  templateCss(): string {
    return css`${BASE_STYLES}${styles}`;
  }

  onConnect(): void {
    this.on(this._refs.head, 'click', () => this.toggle(!this.hasAttribute(OPEN)));
  }

  private toggle(open: boolean): void {
    this._refs.details.hidden = !open;
    this._refs.head.setAttribute('aria-expanded', String(open));
    this.reflect(OPEN, open);
  }

  private renderState(): void {
    const state = this.state.get();
    const meta = toolMeta(state, this.result.get());
    this.reflect('state', state);
    this._refs.meta.textContent = meta.text;
    this._refs.meta.dataset.tone = meta.tone;
    this._refs.stateIcon.name.set(STATE_ICONS[state]);
    this._refs.stateIcon.hidden = STATE_ICONS[state] === '';
  }

  /** A change to a file, a picture, or a subagent at work, shows at once; printed text waits for a click. */
  private renderDisplay(display: ToolDisplay | undefined): void {
    this._refs.diff.hidden = display?.kind !== 'diff';
    this._refs.plan.hidden = display?.kind !== 'plan';
    this._refs.output.hidden = display?.kind !== 'text';
    this._refs.diff.lines.set(display?.kind === 'diff' ? display.lines : []);
    this._refs.plan.steps.set(display?.kind === 'plan' ? [...display.steps] : []);
    this._refs.output.textContent = display?.kind === 'text' ? display.text : '';
    this.renderPicture(display?.kind === 'image' ? display : undefined);
    this.refreshHead();
    this.toggle(display?.kind === 'diff' || display?.kind === 'image' || this.activity.get().length > 0);
  }

  /** A picture the agent made, from the daemon's copy: a click opens it whole in a tab. */
  private renderPicture(picture: { path: string; url: string } | undefined): void {
    this._refs.picture.hidden = picture === undefined;
    if (!picture) {
      return;
    }
    const src = authorizedUrl(picture.url);
    this._refs.picture.href = src;
    this._refs.image.src = src;
    this._refs.image.alt = picture.path;
  }

  private renderActivity(lines: readonly SubagentLine[]): void {
    this._refs.calls.hidden = lines.length === 0;
    this._refs.calls.replaceChildren(...lines.map(callNode));
    this.refreshHead();
    // The first call of a subagent opens the row: its work shows as it happens.
    if (lines.length !== 1) {
      return;
    }
    this.toggle(true);
  }

  private refreshHead(): void {
    const expandable = this.result.get()?.display !== undefined || this.activity.get().length > 0;
    this._refs.head.disabled = !expandable;
    this.reflect('expandable', expandable);
  }
}

ScToolRow.register();
