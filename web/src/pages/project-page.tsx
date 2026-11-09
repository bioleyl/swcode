import { createElement, css } from '@jadis/core';
import {
  confirmDialog,
  JlButton,
  JlCard,
  JlEmpty,
  JlLoadState,
  JlPage,
  JlSwitch,
  JlTabPanel,
  JlTabs,
  toast,
} from '@syware/jlib';

import { ScElement } from '../base/sc-element';
import { placeText } from '../lib/hosts';
import { appDefaults } from '../lib/new-session';
import { presetOf } from '../lib/presets';
import { isSkillOn, SKILL_ORIGIN_LABELS } from '../lib/project-skills';
import { DEFAULT_PROJECT_TAB, isProjectTab, PROJECT_TAB_LABELS } from '../lib/project-tabs';
import { isSwcodeBaseIn } from '../lib/swcode-base';
import { ScSessionChoices } from '../molecules/sc-session-choices';
import { ScSessionRow } from '../molecules/sc-session-row';
import { daemon } from '../services/daemon';
import { daemonState, daemonStatusBus } from '../services/daemon-status';
import { hrefOf, newSessionPath, projectTabPath } from '../services/navigation';
import { presetChoices, presetsBus } from '../services/presets';
import { askToRemoveProject, findProject, projectListBus } from '../services/project-list';
import { foundSkills, readProjectSkills, setProjectSkill } from '../services/project-skills';
import { deleteSession } from '../services/session-delete';
import { allSessions, sessionListBus } from '../services/session-list';
import { installSwcodeBase, readSwcodeBase, removeSwcodeBase, swcodeBaseBus } from '../services/swcode-base';
import { BASE_STYLES } from '../styles/shared';
import styles from './project-page.css?inline';

import type { SessionDefaults } from '../../../shared/protocol/choices.schema';
import type { Project } from '../../../shared/protocol/project.schema';
import type { SessionSummary } from '../../../shared/protocol/session.schema';
import type { ProjectSkill, SwcodeBaseStatus } from '../../../shared/protocol/skill.schema';
import type { ProjectTab } from '../lib/project-tabs';
import type { SwcodeBasePlace, SwcodeBaseRead } from '../types/swcode-base.types';

const APP_NAME = 'swcode';
const NOT_FOUND = 'This project is not on the daemon';
// What the defaults card says it shows: the project's own, or the app's until it has some.
const DEFAULTS_SET = 'Set for this project. The composer still changes them for one session.';
const DEFAULTS_APP =
  'The app’s defaults, until you pick otherwise here. The composer still changes them for one session.';
const SKILLS_NOTE =
  'Read in your ~/.agents/skills and the project’s .agents/skills. The agent is told of those on, and each is a / command in the composer.';
// The card of swcode's own skill: what it says, each place's button, as swcode-base is there or not, and what the
// person agrees to before it leaves a place. The daemon installs and removes it with npx skills, on the project's host.
const SWCODE_BASE_NOTE =
  'swcode’s skill of coding rules, installed with npx skills from bioleyl/swcode. A project finds it in its own .agents/skills, or in your ~/.agents/skills, where every project does.';
const SWCODE_BASE_BUTTONS: Readonly<
  Record<SwcodeBasePlace, Record<'absent' | 'present', { icon: string; label: string }>>
> = {
  home: {
    absent: { icon: 'download', label: 'Install for all projects' },
    present: { icon: 'trash', label: 'Remove from your home' },
  },
  project: {
    absent: { icon: 'download', label: 'Install in this project' },
    present: { icon: 'trash', label: 'Remove from this project' },
  },
};
const SWCODE_BASE_REMOVAL: Readonly<Record<SwcodeBasePlace, { confirm: string; heading: string; text: string }>> =
  {
    home: {
      confirm: 'Remove',
      heading: 'Remove swcode-base from your home?',
      text: 'npx skills deletes it from ~/.agents/skills, which every project reads. A project’s own copy stays.',
    },
    project: {
      confirm: 'Remove',
      heading: 'Remove swcode-base from this project?',
      text: 'npx skills deletes it from the project’s .agents/skills and skills-lock.json. The copy in your home stays.',
    },
  };
const UNREACHABLE = 'The daemon did not answer.';
// A skill's line under its name: where it is, then the start of what it is for.
const MAX_SKILL_LINE = 100;
const ELLIPSIS = '…';
const LINE_SEPARATOR = ' · ';

const clipped = (text: string): string =>
  text.length <= MAX_SKILL_LINE ? text : `${text.slice(0, MAX_SKILL_LINE).trimEnd()}${ELLIPSIS}`;

const skillLineOf = (skill: ProjectSkill): string =>
  [SKILL_ORIGIN_LABELS[skill.origin], clipped(skill.description)]
    .filter((part) => part !== '')
    .join(LINE_SEPARATOR);

/** A failed read, as the skills' or the swcode-base load state tells it. */
class Unread extends Error {}

/**
 * A project, where its folder is, in tabs each with its own address: its sessions, the newest first, each with its
 * delete button; what its new sessions start with (its preset, permissions, thinking and token economy), saved as
 * they are picked; the skills its sessions find, each turned on or off for them with its switch, and swcode-base,
 * put in the project or in the home.
 */
export class ProjectPage extends ScElement {
  static readonly selector = 'project-page';

  readonly route = this.useAttributes({
    id: () => this.render(),
    tab: (tab) => this.showTab(isProjectTab(tab) ? tab : DEFAULT_PROJECT_TAB),
  });

  // The project whose skills are read, or being read: they are read again only for another one.
  private _skillsOf: string | undefined;
  // Where swcode-base is, as the buttons last drew it, so each knows what it does; none while it is read.
  private _swcodeBase: SwcodeBaseStatus | undefined;
  // The card's load state, taken once its shadow is up: ref takes one component per class.
  private _swcodeBaseState?: JlLoadState;
  // Each skill's switch, by its name, as last drawn.
  private readonly _switches = new Map<string, JlSwitch>();

  private readonly _refs = this.useRefs((ref) => ({
    choices: ref(ScSessionChoices),
    defaults: ref<JlCard>('.defaults'),
    empty: ref(JlEmpty),
    list: ref('.sessions'),
    newSession: ref<JlButton>('.new-session'),
    page: ref(JlPage),
    remove: ref<JlButton>('.remove'),
    reset: ref<JlButton>('.reset'),
    skillList: ref('.skill-list'),
    skillsState: ref(JlLoadState),
    swcodeBaseInHome: ref<JlButton>('.swcode-base .in-home'),
    swcodeBaseInProject: ref<JlButton>('.swcode-base .in-project'),
    tabs: ref(JlTabs),
  }));

  templateHtml(): Node {
    return (
      <JlPage appName={APP_NAME} width="standard">
        <JlButton slot="actions" class="remove" icon="trash" variant="ghost" hint="Remove the project" />
        <JlButton slot="actions" class="new-session" icon="plus" variant="primary" hint="New session" />
        <JlTabs label="Project">
          <JlTabPanel
            name="sessions"
            label={PROJECT_TAB_LABELS.sessions.label}
            icon={PROJECT_TAB_LABELS.sessions.icon}>
            <div class="sessions" />
            <JlEmpty
              icon="message"
              heading="No sessions yet"
              description="A new session works in this project’s folder."
            />
          </JlTabPanel>
          <JlTabPanel name="config" label={PROJECT_TAB_LABELS.config.label} icon={PROJECT_TAB_LABELS.config.icon}>
            <JlCard class="defaults" heading="New sessions start with">
              <ScSessionChoices />
              <JlButton class="reset" variant="ghost" size="sm" icon="refresh" label="Use the app’s defaults" />
            </JlCard>
          </JlTabPanel>
          <JlTabPanel name="skills" label={PROJECT_TAB_LABELS.skills.label} icon={PROJECT_TAB_LABELS.skills.icon}>
            <div class="side">
              <JlCard class="skills" heading="Skills" description={SKILLS_NOTE}>
                <JlLoadState skeleton="rows" count={3}>
                  <div class="skill-list" />
                  <JlEmpty
                    slot="empty"
                    icon="book-open"
                    heading="No skills here"
                    description="A skill is a folder with its SKILL.md, in .agents/skills of the project or of your home."
                  />
                </JlLoadState>
              </JlCard>
              <JlCard class="swcode-base" heading="swcode-base" description={SWCODE_BASE_NOTE}>
                <JlLoadState class="base-state" skeleton="rows" count={1}>
                  <div class="base-actions">
                    <JlButton class="in-project" variant="ghost" />
                    <JlButton class="in-home" variant="ghost" />
                  </div>
                </JlLoadState>
              </JlCard>
            </div>
          </JlTabPanel>
        </JlTabs>
      </JlPage>
    );
  }

  templateCss(): string {
    return css`${BASE_STYLES}${styles}`;
  }

  onConnect(): void {
    const { choices } = this._refs;
    this._refs.tabs.events.register('change', (tab) => this.pickTab(tab));
    this._refs.remove.events.register('press', () => askToRemoveProject(this.route.id ?? ''));
    this._refs.reset.events.register('press', () => this.saveDefaults(undefined));
    this._refs.skillsState.events.register('retry', () => this.loadSkills(this.route.id ?? ''));
    this._refs.swcodeBaseInProject.events.register('press', () => void this.changeSwcodeBase('project'));
    this._refs.swcodeBaseInHome.events.register('press', () => void this.changeSwcodeBase('home'));
    choices.events.register('preset', (preset) => this.changeDefault({ preset }));
    choices.events.register('mode', (mode) => this.changeDefault({ mode }));
    choices.events.register('thinking', (thinking) => this.changeDefault({ thinking }));
    choices.events.register('concise', (concise) => this.changeDefault({ concise }));
    this.onBus(projectListBus, 'changed', () => this.render());
    this.onBus(sessionListBus, 'changed', () => this.render());
    this.onBus(presetsBus, 'changed', () => this.render());
    this.onBus(daemonStatusBus, 'changed', () => this.render());
    this.onBus(swcodeBaseBus, 'changed', (read) => this.redrawSwcodeBase(read));
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
    this._refs.tabs.hidden = !project;
    this._refs.newSession.href.set(project ? hrefOf(newSessionPath(project.id)) : '');
    this._refs.newSession.hidden = !project;
    this._refs.remove.hidden = !project;
    const sessions = project ? allSessions().filter((session) => session.workspace === project.path) : [];
    this._refs.list.replaceChildren(...sessions.map((session) => this.rowOf(session)));
    this._refs.empty.hidden = !project || sessions.length > 0;
    if (!project) {
      return;
    }
    this.renderDefaults(project);
    this.renderSkills(project);
    if (this._skillsOf !== project.id) {
      this.forgetSkills();
      this.loadSkills(project.id);
      this.loadSwcodeBase(project.id);
    }
  }

  /** The tab the page shows: the one its address names, or the sessions when it names none yet. */
  private showTab(tab: ProjectTab): void {
    this._refs.tabs.value.set(tab);
  }

  /** A tab picked: the address follows, without leaving the page and what it holds. */
  private pickTab(tab: string): void {
    if (!isProjectTab(tab)) {
      return;
    }
    history.replaceState(history.state, '', hrefOf(projectTabPath(this.route.id ?? '', tab)));
  }

  /** Another project's skills leave at once: its switches would turn them on or off for that one. */
  private forgetSkills(): void {
    this._switches.clear();
    this._refs.skillList.replaceChildren();
    this._refs.skillsState.state.set('loading');
  }

  /** Reads the skills the project's sessions find, on its host: what failed shows in their card, with Try again. */
  private loadSkills(projectId: string): void {
    this._skillsOf = projectId;
    void this._refs.skillsState.load({
      draw: () => this.drawSkills(findProject(projectId)),
      fetch: () => this.fetchSkills(projectId),
      isEmpty: (skills) => skills.length === 0,
    });
  }

  private async fetchSkills(projectId: string): Promise<readonly ProjectSkill[]> {
    const read = await readProjectSkills(projectId);
    if (read === undefined) {
      return this.failSkills(UNREACHABLE);
    }
    if (read.problem !== undefined) {
      return this.failSkills(read.problem);
    }
    return read.skills;
  }

  private failSkills(problem: string): never {
    this._refs.skillsState.errorText.set(problem);
    throw new Unread(problem);
  }

  /** A switch each, as the project has it: a jl-* event does not bubble, so each switch has its own. */
  private drawSkills(project: Project | undefined): void {
    if (!project || project.id !== this._skillsOf) {
      return;
    }
    this._switches.clear();
    for (const skill of foundSkills(project.id)) {
      const toggle = createElement(JlSwitch, {
        attrs: { 'data-skill': skill.name },
        props: { checked: isSkillOn(project, skill.name), description: skillLineOf(skill), label: skill.name },
      });
      toggle.events.register('change', (on) => setProjectSkill(project.id, skill.name, on));
      this._switches.set(skill.name, toggle);
    }
    this._refs.skillList.replaceChildren(...this._switches.values());
  }

  /** The switches follow the project, in place: the one just pressed keeps its focus. */
  private renderSkills(project: Project): void {
    for (const [name, toggle] of this._switches) {
      toggle.checked.set(isSkillOn(project, name));
    }
  }

  /** The card's load state, made with the page: ref takes one component per class, and there are two here. */
  private swcodeBaseState(): JlLoadState {
    this._swcodeBaseState ??= this.getElement<JlLoadState>('jl-load-state.base-state');
    return this._swcodeBaseState;
  }

  /**
   * Reads where swcode-base is for this project: what failed shows in its card, with Try again. Another project's
   * answer leaves at once: its buttons would act on what that one holds.
   */
  private loadSwcodeBase(projectId: string): void {
    this._swcodeBase = undefined;
    void this.swcodeBaseState().load({
      draw: (status: SwcodeBaseStatus) => this.drawSwcodeBase(status),
      fetch: () => this.fetchSwcodeBase(projectId),
    });
  }

  private async fetchSwcodeBase(projectId: string): Promise<SwcodeBaseStatus> {
    const read = await readSwcodeBase(projectId);
    if (read === undefined) {
      return this.failSwcodeBase(UNREACHABLE);
    }
    if (read.problem !== undefined) {
      return this.failSwcodeBase(read.problem);
    }
    return read;
  }

  private failSwcodeBase(problem: string): never {
    this.swcodeBaseState().errorText.set(problem);
    throw new Unread(problem);
  }

  /** Each place's button: install swcode-base where it is not, take it out where it is. */
  private drawSwcodeBase(status: SwcodeBaseStatus): void {
    this._swcodeBase = status;
    this.drawSwcodeBaseButton('project', status);
    this.drawSwcodeBaseButton('home', status);
  }

  private drawSwcodeBaseButton(place: SwcodeBasePlace, status: SwcodeBaseStatus): void {
    const look = SWCODE_BASE_BUTTONS[place][isSwcodeBaseIn(status, place) ? 'present' : 'absent'];
    const button = this.swcodeBaseButton(place);
    button.label.set(look.label);
    button.icon.set(look.icon);
  }

  private swcodeBaseButton(place: SwcodeBasePlace): JlButton {
    const buttons: Record<SwcodeBasePlace, JlButton> = {
      home: this._refs.swcodeBaseInHome,
      project: this._refs.swcodeBaseInProject,
    };
    return buttons[place];
  }

  /**
   * The daemon answered elsewhere. An answer for this project draws the buttons; one for this computer's home alone
   * (the startup offer) is asked again for this project, whose home may be on another host; another project's
   * changes nothing.
   */
  private redrawSwcodeBase(read: SwcodeBaseRead): void {
    const projectId = this.route.id ?? '';
    if (this._swcodeBase === undefined) {
      return;
    }
    if (read.projectId === undefined) {
      void readSwcodeBase(projectId);
      return;
    }
    if (read.projectId === projectId) {
      this.drawSwcodeBase(read.status);
    }
  }

  /** A place's button pressed: swcode-base goes there, or leaves it once the person agreed. */
  private async changeSwcodeBase(place: SwcodeBasePlace): Promise<void> {
    const status = this._swcodeBase;
    if (status === undefined) {
      return;
    }
    const projectId = this.route.id ?? '';
    if (!isSwcodeBaseIn(status, place)) {
      await this.writeSwcodeBase(place, () => installSwcodeBase(place, projectId));
      return;
    }
    const removal = SWCODE_BASE_REMOVAL[place];
    if (!(await confirmDialog({ confirmLabel: removal.confirm, heading: removal.heading, text: removal.text }))) {
      return;
    }
    await this.writeSwcodeBase(place, () => removeSwcodeBase(place, projectId));
  }

  /**
   * Asks the daemon, the place's button turning meanwhile. What it did shows in place: the bus draws the buttons
   * again, and the Skills card is read again, with swcode-base or without it. A failure is a toast.
   */
  private async writeSwcodeBase(
    place: SwcodeBasePlace,
    write: () => Promise<SwcodeBaseStatus | undefined>
  ): Promise<void> {
    const button = this.swcodeBaseButton(place);
    button.loading.set(true);
    const written = await write();
    button.loading.set(false);
    if (written === undefined) {
      toast({ heading: UNREACHABLE, tone: 'danger' });
      return;
    }
    if (written.problem !== undefined) {
      toast({ heading: written.problem, tone: 'danger' });
      return;
    }
    this.loadSkills(this.route.id ?? '');
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
