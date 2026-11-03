import { css } from '@jadis/core';
import { JlAlert, JlButton, JlField, JlPage, JlSelect } from '@syware/jlib';

import { hostLabel } from '../../../shared/hosts/host-key';
import { ScElement } from '../base/sc-element';
import { folderName } from '../lib/format';
import { presetOf, presetStatusOf } from '../lib/presets';
import { preferredProject } from '../lib/project-groups';
import { ScComposer } from '../organisms/sc-composer';
import { daemon, daemonBus } from '../services/daemon';
import { daemonState, daemonStatusBus } from '../services/daemon-status';
import { presetChoices, presetsBus } from '../services/presets';
import { allProjects, askForProject, findProject, projectListBus } from '../services/project-list';
import { allSessions, sessionListBus } from '../services/session-list';
import { BASE_STYLES } from '../styles/shared';
import styles from './home-page.css?inline';

import type { SelectOption } from '@syware/jlib';
import type { Project } from '../../../shared/protocol/project.schema';
import type { PermissionMode, ThinkingMode } from '../../../shared/protocol/session.schema';
import type { PromptDraft } from '../types/composer.types';

const APP_NAME = 'swcode';
// Without a project yet, a session goes in the daemon's folder, which then becomes one. jlib's select takes an empty
// value for nothing chosen: this choice has a value of its own.
const DAEMON_FOLDER = 'daemon-folder';

/** A project in the picker: its name, and its host when it is not this computer. */
const projectLabel = (project: Project): string =>
  project.host.kind === 'local' ? project.name : `${project.name} (${hostLabel(project.host)})`;

const projectOption = (project: Project): SelectOption => ({ label: projectLabel(project), value: project.id });

/** Where a session starts: the project to work in, and the first message, which creates it. */
export class HomePage extends ScElement {
  static readonly selector = 'home-page';

  private _mode: PermissionMode = 'auto';
  // Picked here; otherwise the daemon's default for new sessions.
  private _thinking: ThinkingMode | undefined;
  private _preset: string | undefined;
  private _concise = false;
  // The project in the address (/new/<id>), then the one the person picks.
  private _chosen: string | undefined;

  readonly route = this.useAttributes({
    project: (id) => {
      this._chosen = id ?? undefined;
      this.renderProjects();
    },
  });

  private readonly _refs = this.useRefs((ref) => ({
    composer: ref(ScComposer),
    newProject: ref<JlButton>('.new-project'),
    problem: ref(JlAlert),
    project: ref(JlSelect),
  }));

  templateHtml(): Node {
    return (
      <JlPage
        heading="New session"
        description="Tell swcode what to work on: it works in the project’s folder."
        appName={APP_NAME}
        width="form">
        <div class="where">
          <JlField label="Project">
            <JlSelect />
          </JlField>
          <JlButton class="new-project" icon="folder-plus" label="New project" />
        </div>
        <ScComposer />
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
    composer.events.register('mode', (mode) => {
      this._mode = mode;
    });
    composer.events.register('thinking', (thinking) => {
      this._thinking = thinking;
    });
    composer.events.register('concise', (concise) => {
      this._concise = concise;
    });
    composer.events.register('preset', (preset) => {
      this._preset = preset;
      this.renderProjects();
    });
    this._refs.newProject.events.register('press', () => askForProject());
    this._refs.project.events.register('change', (value) => {
      this._chosen = value;
    });
    this.onBus(daemonStatusBus, 'changed', () => this.renderProjects());
    this.onBus(projectListBus, 'changed', () => this.renderProjects());
    this.onBus(sessionListBus, 'changed', () => this.renderProjects());
    this.onBus(presetsBus, 'changed', () => this.renderProjects());
    this.onBus(daemonBus, 'message', (message) => {
      if (message.type !== 'problem') {
        return;
      }
      this.showProblem(message.message);
    });
    this.renderProjects();
    composer.focusText();
  }

  /** Creates the session and starts it on the message; the app opens it once the daemon made it. */
  private start(draft: PromptDraft): void {
    this.showProblem('');
    const project = findProject(this._refs.project.value.get());
    daemon.send({
      concise: this._concise,
      mode: this._mode,
      prompt: draft.text,
      type: 'createSession',
      ...(draft.pictures.length === 0 ? {} : { pictures: draft.pictures }),
      ...(this._thinking === undefined ? {} : { thinking: this._thinking }),
      ...(this._preset === undefined ? {} : { preset: this._preset }),
      ...(project === undefined ? {} : { host: project.host, workspace: project.path }),
    });
  }

  private renderProjects(): void {
    const state = daemonState();
    const projects = allProjects();
    const selected = preferredProject(projects, allSessions(), this._chosen);
    const daemonFolder = state.kind === 'ready' ? folderName(state.status.workspace) : 'the daemon’s folder';
    this._refs.project.options.set(
      projects.length === 0 ? [{ label: daemonFolder, value: DAEMON_FOLDER }] : projects.map(projectOption)
    );
    this._refs.project.value.set(selected?.id ?? DAEMON_FOLDER);
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

HomePage.register();
