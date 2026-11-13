import { createElement, css } from '@jadis/core';
import { JlAlert, JlButton, JlCard, JlField, JlPage, JlSelect } from '@syware/jlib';

import { hostLabel } from '../../../shared/hosts/host-key';
import { ScElement } from '../base/sc-element';
import { NEW_DRAFT_PLACES } from '../lib/drafts';
import { folderName } from '../lib/format';
import { appDefaults, CONCISE_AT_FIRST, startingChoices } from '../lib/new-session';
import { presetOf, presetStatusOf } from '../lib/presets';
import { preferredProject } from '../lib/project-groups';
import { skillsOn } from '../lib/project-skills';
import { sessionsOf } from '../lib/session-kinds';
import { ScTodoRow } from '../molecules/sc-todo-row';
import { ScComposer } from '../organisms/sc-composer';
import { daemon, daemonBus } from '../services/daemon';
import { daemonState, daemonStatusBus } from '../services/daemon-status';
import { presetChoices, presetsBus } from '../services/presets';
import { allProjects, askForProject, findProject, projectListBus } from '../services/project-list';
import { foundSkills, projectSkillsBus, readProjectSkills } from '../services/project-skills';
import { allSessions, sessionListBus } from '../services/session-list';
import { BASE_STYLES } from '../styles/shared';
import styles from './home-page.css?inline';

import type { SelectOption } from '@syware/jlib';
import type { SessionDefaults } from '../../../shared/protocol/choices.schema';
import type { Project, ProjectTodo } from '../../../shared/protocol/project.schema';
import type { PromptDraft } from '../types/composer.types';

const APP_NAME = 'swcode';
// Without a project yet, a session goes in the daemon's folder, which then becomes one. jlib's select takes an empty
// value for nothing chosen: this choice has a value of its own.
const DAEMON_FOLDER = 'daemon-folder';
const TODOS_NOTE = 'The ones you tick are given to the agent with your first message.';

/** A project in the picker: its name, and its host when it is not this computer. */
const projectLabel = (project: Project): string =>
  project.host.kind === 'local' ? project.name : `${project.name} (${hostLabel(project.host)})`;

const projectOption = (project: Project): SelectOption => ({ label: projectLabel(project), value: project.id });

/**
 * Where a session starts: the project to work in, and the first message, which creates it; the composer's / commands
 * are the skills that project has on.
 */
export class HomePage extends ScElement {
  static readonly selector = 'home-page';

  // What the person picked here, for this session alone: the project's defaults, then the app's, give the rest.
  private _picks: Partial<SessionDefaults> = {};
  // The project in the address (/new/<id>), then the one the person picks.
  private _chosen: string | undefined;
  // The project whose skills this page read for the composer: they are read again for another one.
  private _skillsOf: string | undefined;
  // The project whose todos are shown, and the ones the person pinned to this session.
  private _todosOf: string | undefined;
  private _pinned = new Set<string>();

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
    todoList: ref('.todo-list'),
    todos: ref(JlCard),
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
        <JlCard class="todos" heading="Todos" description={TODOS_NOTE} hidden>
          <div class="todo-list" />
        </JlCard>
        <ScComposer concise={CONCISE_AT_FIRST} draftPlace={NEW_DRAFT_PLACES.code} />
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
    composer.events.register('mode', (mode) => this.pick({ mode }));
    composer.events.register('thinking', (thinking) => this.pick({ thinking }));
    composer.events.register('concise', (concise) => this.pick({ concise }));
    composer.events.register('preset', (preset) => this.pick({ preset }));
    this._refs.newProject.events.register('press', () => askForProject());
    this._refs.project.events.register('change', (value) => {
      this._chosen = value;
      this.renderChoices();
    });
    this.onBus(daemonStatusBus, 'changed', () => this.renderProjects());
    this.onBus(projectListBus, 'changed', () => this.renderProjects());
    this.onBus(sessionListBus, 'changed', () => this.renderProjects());
    this.onBus(presetsBus, 'changed', () => this.renderProjects());
    this.onBus(projectSkillsBus, 'changed', () => this.renderSkills());
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
    const choices = this.choicesFor(project);
    const pinned = this.pinnedTodos(project);
    daemon.send({
      concise: choices.concise,
      mode: choices.mode,
      prompt: draft.text,
      thinking: choices.thinking,
      type: 'createSession',
      ...(draft.pictures.length === 0 ? {} : { pictures: draft.pictures }),
      ...(choices.preset === '' ? {} : { preset: choices.preset }),
      ...(pinned.length === 0
        ? {}
        : { pinnedTodoIds: pinned.map((todo) => todo.id), pinnedTodos: pinned.map((todo) => todo.text) }),
      ...(project === undefined ? {} : { host: project.host, workspace: project.path }),
    });
  }

  /** The project's open todos the person pinned, in the order the project lists them. */
  private pinnedTodos(project: Project | undefined): ProjectTodo[] {
    if (!project) {
      return [];
    }
    return project.todos.filter((todo) => !todo.done && this._pinned.has(todo.id));
  }

  private pick(choice: Partial<SessionDefaults>): void {
    this._picks = { ...this._picks, ...choice };
    this.renderChoices();
  }

  /** What the session starts with in a project: the person's picks, its defaults, the app's. */
  private choicesFor(project: Project | undefined): SessionDefaults {
    const state = daemonState();
    const app = appDefaults(state.kind === 'ready' ? state.status.thinking : undefined);
    return startingChoices(app, project?.defaults, this._picks);
  }

  private renderProjects(): void {
    const state = daemonState();
    const projects = allProjects();
    const selected = preferredProject(projects, sessionsOf('code', allSessions()), this._chosen);
    const daemonFolder = state.kind === 'ready' ? folderName(state.status.workspace) : 'the daemon’s folder';
    this._refs.project.options.set(
      projects.length === 0 ? [{ label: daemonFolder, value: DAEMON_FOLDER }] : projects.map(projectOption)
    );
    this._refs.project.value.set(selected?.id ?? DAEMON_FOLDER);
    this.renderChoices();
  }

  /** The composer's choices, as the session would start with them in the project chosen. */
  private renderChoices(): void {
    const state = daemonState();
    const choices = this.choicesFor(findProject(this._refs.project.value.get()));
    const main = presetStatusOf(
      state.kind === 'ready' ? state.status : undefined,
      presetOf(presetChoices(), choices.preset)
    );
    const composer = this._refs.composer;
    composer.seesPictures.set(main?.vision ?? false);
    composer.preset.set(choices.preset);
    composer.mode.set(choices.mode);
    composer.thinking.set(choices.thinking);
    composer.concise.set(choices.concise);
    this.renderSkills();
    this.renderTodos();
  }

  /** The skills of the project chosen, which the first message may run: read on its host once it is chosen. */
  private renderSkills(): void {
    const project = findProject(this._refs.project.value.get());
    this._refs.composer.skills.set(project ? skillsOn(foundSkills(project.id), project) : []);
    if (!project || project.id === this._skillsOf) {
      return;
    }
    this._skillsOf = project.id;
    void readProjectSkills(project.id);
  }

  /** The open todos of the project chosen, each pinned or not; another project's pins are let go at once. */
  private renderTodos(): void {
    const project = findProject(this._refs.project.value.get());
    const open = project?.todos.filter((todo) => !todo.done) ?? [];
    if (project?.id !== this._todosOf) {
      this._pinned.clear();
      this._todosOf = project?.id;
    }
    this._refs.todos.hidden = open.length === 0;
    this._refs.todoList.replaceChildren(...open.map((todo) => this.pinRowOf(todo)));
  }

  private pinRowOf(todo: ProjectTodo): Node {
    const row = createElement(ScTodoRow, { props: { checked: this._pinned.has(todo.id), todo } });
    row.events.register('change', (checked) => this.pin(todo.id, checked));
    return row;
  }

  private pin(todoId: string, pinned: boolean): void {
    pinned ? this._pinned.add(todoId) : this._pinned.delete(todoId);
  }

  /** What the daemon refused, under the message: nothing once a new one goes. */
  private showProblem(text: string): void {
    this._refs.problem.description.set(text);
    this._refs.problem.hidden = text === '';
  }
}

HomePage.register();
