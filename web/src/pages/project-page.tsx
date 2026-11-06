import { createElement, css } from '@jadis/core';
import { JlButton, JlCard, JlEmpty, JlPage } from '@syware/jlib';

import { ScElement } from '../base/sc-element';
import { placeText } from '../lib/hosts';
import { appDefaults } from '../lib/new-session';
import { presetOf } from '../lib/presets';
import { ScSessionChoices } from '../molecules/sc-session-choices';
import { ScSessionRow } from '../molecules/sc-session-row';
import { daemon } from '../services/daemon';
import { daemonState, daemonStatusBus } from '../services/daemon-status';
import { hrefOf, newSessionPath } from '../services/navigation';
import { presetChoices, presetsBus } from '../services/presets';
import { askToRemoveProject, findProject, projectListBus } from '../services/project-list';
import { deleteSession } from '../services/session-delete';
import { allSessions, sessionListBus } from '../services/session-list';
import { BASE_STYLES } from '../styles/shared';
import styles from './project-page.css?inline';

import type { SessionDefaults } from '../../../shared/protocol/choices.schema';
import type { Project } from '../../../shared/protocol/project.schema';
import type { SessionSummary } from '../../../shared/protocol/session.schema';

const APP_NAME = 'swcode';
const NOT_FOUND = 'This project is not on the daemon';
// What the defaults card says it shows: the project's own, or the app's until it has some.
const DEFAULTS_SET = 'Set for this project. The composer still changes them for one session.';
const DEFAULTS_APP =
  'The app’s defaults, until you pick otherwise here. The composer still changes them for one session.';

/**
 * A project: where its folder is, its sessions, the newest first, each with its delete button, and what its new
 * sessions start with (its preset, permissions, thinking and token economy), saved as they are picked.
 */
export class ProjectPage extends ScElement {
  static readonly selector = 'project-page';

  readonly route = this.useAttributes({ id: () => this.render() });

  private readonly _refs = this.useRefs((ref) => ({
    choices: ref(ScSessionChoices),
    defaults: ref(JlCard),
    empty: ref(JlEmpty),
    list: ref('.sessions'),
    newSession: ref<JlButton>('.new-session'),
    page: ref(JlPage),
    remove: ref<JlButton>('.remove'),
    reset: ref<JlButton>('.reset'),
  }));

  templateHtml(): Node {
    return (
      <JlPage appName={APP_NAME} width="standard">
        <JlButton slot="actions" class="remove" icon="trash" variant="ghost" label="Remove" />
        <JlButton slot="actions" class="new-session" icon="plus" variant="primary" label="New session" />
        <div class="layout">
          <div class="column">
            <div class="sessions" />
            <JlEmpty
              icon="message"
              heading="No sessions yet"
              description="A new session works in this project’s folder."
            />
          </div>
          <JlCard class="defaults" heading="New sessions start with">
            <ScSessionChoices />
            <JlButton class="reset" variant="ghost" size="sm" icon="refresh" label="Use the app’s defaults" />
          </JlCard>
        </div>
      </JlPage>
    );
  }

  templateCss(): string {
    return css`${BASE_STYLES}${styles}`;
  }

  onConnect(): void {
    const { choices } = this._refs;
    this._refs.remove.events.register('press', () => askToRemoveProject(this.route.id ?? ''));
    this._refs.reset.events.register('press', () => this.saveDefaults(undefined));
    choices.events.register('preset', (preset) => this.changeDefault({ preset }));
    choices.events.register('mode', (mode) => this.changeDefault({ mode }));
    choices.events.register('thinking', (thinking) => this.changeDefault({ thinking }));
    choices.events.register('concise', (concise) => this.changeDefault({ concise }));
    this.onBus(projectListBus, 'changed', () => this.render());
    this.onBus(sessionListBus, 'changed', () => this.render());
    this.onBus(presetsBus, 'changed', () => this.render());
    this.onBus(daemonStatusBus, 'changed', () => this.render());
    this.render();
  }

  private project(): Project | undefined {
    return findProject(this.route.id ?? '');
  }

  /** What the project's new sessions start with now: its own defaults, else the app's. */
  private defaultsOf(project: Project): SessionDefaults {
    const state = daemonState();
    return project.defaults ?? appDefaults(state.kind === 'ready' ? state.status.thinking : undefined);
  }

  /** One choice changed: the project keeps all four from then on, the others as they showed. */
  private changeDefault(choice: Partial<SessionDefaults>): void {
    const project = this.project();
    if (!project) {
      return;
    }
    this.saveDefaults({ ...this.defaultsOf(project), ...choice });
  }

  private saveDefaults(defaults: SessionDefaults | undefined): void {
    const projectId = this.route.id ?? '';
    daemon.send({ projectId, type: 'setProjectDefaults', ...(defaults === undefined ? {} : { defaults }) });
  }

  private render(): void {
    const project = this.project();
    this._refs.page.heading.set(project?.name ?? NOT_FOUND);
    this._refs.page.description.set(project ? placeText(project.host, project.path) : '');
    this._refs.newSession.href.set(project ? hrefOf(newSessionPath(project.id)) : '');
    this._refs.newSession.hidden = !project;
    this._refs.remove.hidden = !project;
    this._refs.defaults.hidden = !project;
    const sessions = project ? allSessions().filter((session) => session.workspace === project.path) : [];
    this._refs.list.replaceChildren(...sessions.map((session) => this.rowOf(session)));
    this._refs.empty.hidden = !project || sessions.length > 0;
    if (!project) {
      return;
    }
    this.renderDefaults(project);
  }

  /** A session's row, its delete button asking first: a jl-* event does not bubble, so each row has its own. */
  private rowOf(session: SessionSummary): Node {
    const row = createElement(ScSessionRow, { props: { removable: true, session } });
    row.events.register('remove', (removed) => void deleteSession(removed));
    return row;
  }

  /** The choices a new session starts with, the default preset shown as itself; whose they are, and the way back. */
  private renderDefaults(project: Project): void {
    const defaults = this.defaultsOf(project);
    const presets = presetChoices();
    const panel = this._refs.choices;
    panel.choices.set(presets);
    panel.preset.set(presetOf(presets, defaults.preset)?.id ?? '');
    panel.mode.set(defaults.mode);
    panel.thinking.set(defaults.thinking);
    panel.concise.set(defaults.concise);
    this._refs.defaults.description.set(project.defaults ? DEFAULTS_SET : DEFAULTS_APP);
    this._refs.reset.hidden = !project.defaults;
  }
}

ProjectPage.register();
