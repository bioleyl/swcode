import { css } from '@jadis/core';
import { JlAlert, JlButton, JlCard, JlFacts, JlLink, JlSwitch } from '@syware/jlib';

import { ScElement } from '../base/sc-element';
import { INSTALLED_TEXTS, updateFacts, updateNote } from '../lib/update-view';
import { ScMessage } from '../molecules/sc-message';
import { checkNow, installUpdate, loadUpdate, setChecks } from '../services/updates';
import { BASE_STYLES } from '../styles/shared';
import styles from './sc-update-settings.css?inline';

import type { Tone } from '@syware/jlib';
import type { UpdateState, UpdateView } from '../../../shared/protocol/update.schema';

const NOT_SAVED = 'swcode did not take it: nothing changed.';
const UNREACHABLE = 'The daemon did not answer.';

/**
 * The settings' Updates: the version that runs, whether swcode looks for a newer one every 10 minutes, looking now,
 * and the newer one found, with what it brings, to install. A computer shows it, not a phone.
 */
export class ScUpdateSettings extends ScElement {
  static readonly selector = 'sc-update-settings';

  private readonly _refs = this.useRefs((ref) => ({
    check: ref<JlButton>('.check'),
    facts: ref(JlFacts),
    feedback: ref(JlAlert),
    install: ref<JlButton>('.install'),
    looking: ref(JlSwitch),
    notes: ref(ScMessage),
    page: ref(JlLink),
    release: ref(JlCard),
  }));

  templateHtml(): Node {
    return (
      <div class="updates">
        <p class="lead">
          swcode looks for a newer version on GitHub, and asks on a computer before installing it. Installing
          restarts swcode; the sessions keep their history.
        </p>
        <JlFacts label="Versions" />
        <JlSwitch label="Look every 10 minutes" description="For a release newer than this one, on GitHub." />
        <div class="row">
          <JlButton class="check" icon="refresh" label="Look now" />
        </div>
        <JlAlert hidden />
        <JlCard class="release" hidden>
          <JlButton slot="actions" class="install" variant="primary" label="Install" />
          <ScMessage />
          <JlLink slot="footer" label="Its page on GitHub" external />
        </JlCard>
      </div>
    );
  }

  templateCss(): string {
    return css`${BASE_STYLES}${styles}`;
  }

  onConnect(): void {
    this._refs.looking.events.register('change', (on) => void this.turn(on));
    this._refs.check.events.register('press', () => void this.look());
    this._refs.install.events.register('press', () => void this.install());
    void this.load();
  }

  private async load(): Promise<void> {
    const view = await loadUpdate();
    if (!view || !this.isConnected) {
      return;
    }
    this.render(view);
  }

  private render(view: UpdateView): void {
    const fromSources = view.state.kind === 'source';
    this._refs.facts.items.set(updateFacts(view, new Date()));
    this._refs.looking.checked.set(view.checks);
    this._refs.looking.disabled.set(fromSources);
    this._refs.check.disabled.set(fromSources);
    const note = updateNote(view);
    this.say(note?.text ?? '', note?.tone ?? 'info');
    this.renderRelease(view.state);
  }

  /** The newer version found, what it brings, and its Install. */
  private renderRelease(state: UpdateState): void {
    this._refs.release.hidden = state.kind !== 'newer';
    if (state.kind !== 'newer') {
      return;
    }
    this._refs.release.heading.set(`swcode ${state.release.version}`);
    this._refs.install.label.set(`Install ${state.release.version}`);
    this._refs.notes.text.set(state.release.notes);
    this._refs.page.href.set(state.release.page);
  }

  private say(text: string, tone: Tone): void {
    this._refs.feedback.tone.set(tone);
    this._refs.feedback.description.set(text);
    this._refs.feedback.hidden = text === '';
  }

  private async turn(on: boolean): Promise<void> {
    const view = await setChecks(on);
    if (!this.isConnected) {
      return;
    }
    if (!view) {
      this.say(NOT_SAVED, 'danger');
      await this.load();
      return;
    }
    this.render(view);
  }

  private async look(): Promise<void> {
    this._refs.check.loading.set(true);
    const view = await checkNow();
    this._refs.check.loading.set(false);
    if (!this.isConnected) {
      return;
    }
    if (!view) {
      this.say(UNREACHABLE, 'danger');
      return;
    }
    this.render(view);
  }

  private async install(): Promise<void> {
    this._refs.install.loading.set(true);
    const installed = await installUpdate();
    this._refs.install.loading.set(false);
    if (!this.isConnected) {
      return;
    }
    if (!installed.ok) {
      this.say(installed.problem, 'danger');
      return;
    }
    this._refs.release.hidden = true;
    this.say(INSTALLED_TEXTS[installed.restart](installed.version), 'ok');
  }
}

ScUpdateSettings.register();
