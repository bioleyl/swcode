import { css } from '@jadis/core';
import { JlAlert, JlPage } from '@syware/jlib';

import { ScElement } from '../base/sc-element';
import { NEW_DRAFT_PLACES } from '../lib/drafts';
import { CONCISE_AT_FIRST } from '../lib/new-session';
import { presetOf, presetStatusOf } from '../lib/presets';
import { ScComposer } from '../organisms/sc-composer';
import { daemon, daemonBus } from '../services/daemon';
import { daemonState, daemonStatusBus } from '../services/daemon-status';
import { presetChoices, presetsBus } from '../services/presets';
import { BASE_STYLES } from '../styles/shared';
import styles from './new-chat-page.css?inline';

import type { ThinkingMode } from '../../../shared/protocol/session.schema';
import type { PromptDraft } from '../types/composer.types';

const APP_NAME = 'swcode';

/** Where a chat starts: its first message, which creates it. A chat has no project: the model answers, reading the web. */
export class NewChatPage extends ScElement {
  static readonly selector = 'new-chat-page';

  // Picked here; otherwise the daemon's default for new sessions.
  private _thinking: ThinkingMode | undefined;
  private _preset: string | undefined;
  private _concise = CONCISE_AT_FIRST;

  private readonly _refs = this.useRefs((ref) => ({
    composer: ref(ScComposer),
    problem: ref(JlAlert),
  }));

  templateHtml(): Node {
    return (
      <JlPage
        heading="New chat"
        description="Ask anything: swcode answers, and searches the web when it needs to."
        appName={APP_NAME}
        width="form">
        <ScComposer kind="chat" concise={CONCISE_AT_FIRST} draftPlace={NEW_DRAFT_PLACES.chat} />
        <JlAlert tone="danger" hidden />
      </JlPage>
    );
  }

  templateCss(): string {
    return css`${BASE_STYLES}${styles}`;
  }

  onConnect(): void {
    const composer = this._refs.composer;
    composer.events.register('send', (draft) => this.start(draft));
    composer.events.register('thinking', (thinking) => {
      this._thinking = thinking;
    });
    composer.events.register('concise', (concise) => {
      this._concise = concise;
    });
    composer.events.register('preset', (preset) => {
      this._preset = preset;
      this.renderModel();
    });
    this.onBus(daemonStatusBus, 'changed', () => this.renderModel());
    this.onBus(presetsBus, 'changed', () => this.renderModel());
    this.onBus(daemonBus, 'message', (message) => {
      if (message.type !== 'problem') {
        return;
      }
      this.showProblem(message.message);
    });
    this.renderModel();
    composer.focusText();
  }

  /** Creates the chat and starts it on the message; the app opens it once the daemon made it. */
  private start(draft: PromptDraft): void {
    this.showProblem('');
    daemon.send({
      concise: this._concise,
      kind: 'chat',
      prompt: draft.text,
      type: 'createSession',
      ...(draft.pictures.length === 0 ? {} : { pictures: draft.pictures }),
      ...(this._thinking === undefined ? {} : { thinking: this._thinking }),
      ...(this._preset === undefined ? {} : { preset: this._preset }),
    });
  }

  /** What the preset's main model is, as its llm-server says now: whether it sees pictures, how much it thinks. */
  private renderModel(): void {
    const state = daemonState();
    const preset = presetOf(presetChoices(), this._preset);
    const main = presetStatusOf(state.kind === 'ready' ? state.status : undefined, preset);
    this._refs.composer.seesPictures.set(main?.vision ?? false);
    const thinking = state.kind === 'ready' ? state.status.thinking : undefined;
    this._refs.composer.thinking.set(this._thinking ?? thinking ?? 'full');
  }

  /** What the daemon refused, under the message: nothing once a new one goes. */
  private showProblem(text: string): void {
    this._refs.problem.description.set(text);
    this._refs.problem.hidden = text === '';
  }
}

NewChatPage.register();
