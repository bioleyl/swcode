import { css } from '@jadis/core';
import { JlIcon } from '@syware/jlib';

import { ScElement } from '../base/sc-element';
import { lastLines } from '../lib/format';
import { absolutePath, pathOfCall } from '../lib/ide-open';
import { toolLabel } from '../lib/tool-labels';
import { toolMeta } from '../lib/tool-meta';
import { authorizedUrl } from '../services/access';
import { requestOpenInHost } from '../services/host-messages';
import { BASE_STYLES } from '../styles/shared';
import { ScDiff } from './sc-diff';
import { ScPlanSteps } from './sc-plan-steps';
import styles from './sc-tool-row.css?inline';

import type { DecisionDisplay, ToolDisplay, ToolName } from '../../../shared/protocol/session.schema';
import type { SubagentLine, ToolResult, ToolRowState } from '../types/conversation.types';

const OPEN = 'open';
const CALL_ICON_SIZE = 14;
const OPEN_ICON = 'open';
const OPEN_HINT = 'Open the file in the editor';
// What a running command shows of what it printed: its last lines, from the end of what came.
const LIVE_LINES = 8;

/** A choice the model recorded, as its row shows it when opened. */
const decisionText = (display: DecisionDisplay): string => {
  const lines = [`Chosen: ${display.chosen}`, `Options: ${display.options.join(', ')}`];
  if (display.why !== undefined && display.why !== '') {
    lines.push(`Why: ${display.why}`);
  }
  return lines.join('\n');
};
const LIVE_CHARS = 4000;

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
  queued: '',
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
    this.renderOpen();
  });
  readonly summary = this.useChange('', (value) => {
    this._refs.summary.textContent = value;
    this._refs.summary.title = value;
    this.renderOpen();
  });
  /** The session's project folder: a tool's path is relative to it, so the host is given a whole one. */
  readonly projectPath = this.useChange('', () => this.renderOpen());
  readonly state = this.useChange<ToolRowState>('pending', () => this.renderState());
  readonly result = this.useChange<ToolResult | undefined>(undefined, (result) => {
    this.renderState();
    this.renderDisplay(result?.display);
    // The result holds all the command printed: what showed while it ran goes.
    this._live = '';
    this.renderLive();
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
    live: ref('.live'),
    meta: ref('.meta-text'),
    name: ref('.name'),
    open: ref<JlIcon>('.open'),
    output: ref('.details pre'),
    picture: ref('a'),
    plan: ref<ScPlanSteps>('sc-plan-steps'),
    stateIcon: ref<JlIcon>('.state-icon'),
    summary: ref('.summary'),
  }));

  // The end of what the call's command printed so far, while it runs.
  private _live = '';

  /** What the call's command printed since the last time: the row shows the end of it until the result comes. */
  printed(text: string): void {
    this._live = `${this._live}${text}`.slice(-LIVE_CHARS);
    this.renderLive();
  }

  private renderLive(): void {
    this._refs.live.textContent = lastLines(this._live, LIVE_LINES);
    this._refs.live.hidden = this._live === '';
  }

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
          <JlIcon class="open" name={OPEN_ICON} size={14} label={OPEN_HINT} />
          <JlIcon class="chevron" name="chevron-down" size={14} />
        </button>
        <pre class="live" aria-live="off" hidden />
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
    this.on(this._refs.open, 'click', (event) => this.openFromHead(event));
    this._refs.diff.events.register('open', (place) => this.openAt(place.line));
  }

  /** The Open icon asks the host for the file; the head's own click opens and closes the row. */
  private openFromHead(event: MouseEvent): void {
    event.stopPropagation();
    this.openAt();
  }

  /** The Open icon shows for the calls that concern a file: a change to one, or a read of one. */
  private renderOpen(): void {
    this.reflect('file', this.openPath() !== '');
  }

  /** The file the call concerns: the change's path, else the path a file tool's summary names. */
  private openPath(): string {
    return pathOfCall(this.tool.get(), this.summary.get(), this.result.get()?.display);
  }

  /** Asks the host to open the file the call concerns, at a line when one is given. */
  private openAt(line?: number): void {
    const path = this.openPath();
    if (path !== '') {
      requestOpenInHost(absolutePath(this.projectPath.get(), path), line, line);
    }
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

  /**
   * A change to a file or a picture shows at once; printed text, and a subagent's calls and report, wait for a click:
   * several subagents at work would fill the conversation.
   */
  private renderDisplay(display: ToolDisplay | undefined): void {
    this._refs.diff.hidden = display?.kind !== 'diff';
    this._refs.plan.hidden = display?.kind !== 'plan';
    const text =
      display?.kind === 'text' ? display.text : display?.kind === 'decision' ? decisionText(display) : '';
    this._refs.output.hidden = text === '';
    this._refs.diff.lines.set(display?.kind === 'diff' ? display.lines : []);
    this._refs.plan.steps.set(display?.kind === 'plan' ? [...display.steps] : []);
    this._refs.output.textContent = text;
    this.renderPicture(display?.kind === 'image' ? display : undefined);
    this.renderOpen();
    this.refreshHead();
    this.toggle(display?.kind === 'diff' || display?.kind === 'image');
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
  }

  private refreshHead(): void {
    const expandable = this.result.get()?.display !== undefined || this.activity.get().length > 0;
    this._refs.head.disabled = !expandable;
    this.reflect('expandable', expandable);
  }
}

ScToolRow.register();
