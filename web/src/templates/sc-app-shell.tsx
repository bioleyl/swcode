import { css, Router } from '@jadis/core';
import {
  confirmDialog,
  JlAlert,
  JlAppShell,
  JlCommandPalette,
  JlPageOutlet,
  JlSearchButton,
  JlTabBar,
  JlToaster,
} from '@syware/jlib';

import { ScBrand } from '../atoms/sc-brand';
import { ScElement } from '../base/sc-element';
import { daemonLine } from '../lib/daemon-lines';
import { matchCommands, PALETTE_ACTION, paletteCommands, paletteTarget } from '../lib/palette';
import { removalConsequence, sessionInProject, sessionsInProject } from '../lib/project-groups';
import { PHONE_TOPIC, SETTINGS_TOPICS } from '../lib/settings-topics';
import { NAV_ID, projectNavId, ScNav, sessionNavId } from '../organisms/sc-nav';
import { ScProjectDialog } from '../organisms/sc-project-dialog';
import { ROUTES } from '../routes';
import { daemon } from '../services/daemon';
import { daemonState, daemonStatusBus } from '../services/daemon-status';
import { inDesktopApp } from '../services/desktop';
import {
  HOME_PATH,
  hrefOf,
  NEW_CHAT_PATH,
  NEW_SESSION_PATH,
  navigate,
  newPathOf,
  newSessionPath,
  projectPath,
  redirect,
  SESSIONS_PATH,
  SETTINGS_PATH,
  sessionPath,
  settingsPath,
} from '../services/navigation';
import {
  allProjects,
  askForProject,
  findProject,
  projectListBus,
  setProjectTrust,
} from '../services/project-list';
import { seenBus, seenSessions } from '../services/seen-sessions';
import { allSessions, findSession, sessionListBus } from '../services/session-list';
import { revealInSession } from '../services/session-reveal';
import { searchSessions, sessionSearchBus } from '../services/session-search';
import { loadStartup } from '../services/startup';
import { offerSwcodeBase, readSwcodeBase } from '../services/swcode-base';
import { offerUpdate } from '../services/update-offer';
import { installsUpdates } from '../services/updates';
import { setWorkMode, workMode, workModeBus } from '../services/work-mode';
import { BASE_STYLES } from '../styles/shared';
import styles from './sc-app-shell.css?inline';

import type { ActiveRoute } from '@jadis/core';
import type { PaletteCommand, TabBarItem } from '@syware/jlib';
import type { Project } from '../../../shared/protocol/project.schema';
import type { SessionKind } from '../../../shared/protocol/session.schema';
import type { SettingsTopic } from '../lib/settings-topics';
import type { RouteName } from '../routes';
import type { DaemonState } from '../types/daemon.types';
import type { PaletteTarget, PaletteTargetMap } from '../types/palette.types';

const HASH_CHANGE = 'hashchange';
const TRUST_QUESTION =
  'Its AGENTS.md and its own skills are instructions the agent follows as if they were yours. Trust a folder whose files you know. Otherwise they are kept from the agent and its commands keep their sandbox, until its Config tab says you trust it.';
// A window as narrow as a phone's, where the session page puts its composer along the bottom.
const PHONE_QUERY = '(max-width: 720px)';

/** A phone's tabs, under the page: a new session or chat, every session, the settings; More opens the navigation. */
const TAB = { new: 'new', sessions: 'sessions', settings: 'settings' } as const;
const tabsFor = (mode: SessionKind): TabBarItem[] => [
  { href: hrefOf(newPathOf(mode)), icon: 'plus', id: TAB.new, label: 'New' },
  { href: hrefOf(SESSIONS_PATH), icon: 'message', id: TAB.sessions, label: 'Sessions' },
  { href: hrefOf(SETTINGS_PATH), icon: 'settings', id: TAB.settings, label: 'Settings' },
];

/** Where a page is: its navigation item, its phone tab, and the mode it belongs to, none for a page of both. */
interface Place {
  nav: string;
  tab: string;
  mode?: SessionKind;
}

/**
 * The addresses with no page of their own (swcode's link, `/`, an address it does not know): they lead to the new
 * page of the mode left last, so the app opens as the person left it.
 */
const LEADING_ON: ReadonlySet<string> = new Set<RouteName>(['home', 'notFound']);

/**
 * The place of each page: a new page does not compile until it says. A session's mode is its kind, once listed; the
 * addresses that lead on are never shown.
 */
const PLACES: Readonly<Record<RouteName, (params: Record<string, string>) => Place>> = {
  home: () => ({ nav: '', tab: '' }),
  newChat: () => ({ mode: 'chat', nav: NAV_ID.newChat, tab: TAB.new }),
  newInProject: () => ({ mode: 'code', nav: NAV_ID.newSession, tab: TAB.new }),
  newSession: () => ({ mode: 'code', nav: NAV_ID.newSession, tab: TAB.new }),
  notFound: () => ({ nav: '', tab: '' }),
  project: (params) => ({ mode: 'code', nav: projectNavId(params.id ?? ''), tab: TAB.sessions }),
  projectTab: (params) => ({ mode: 'code', nav: projectNavId(params.id ?? ''), tab: TAB.sessions }),
  session: (params) => ({
    mode: findSession(params.id ?? '')?.kind,
    nav: sessionNavId(params.id ?? ''),
    tab: TAB.sessions,
  }),
  sessions: () => ({ nav: '', tab: TAB.sessions }),
  settings: () => ({ nav: '', tab: TAB.settings }),
  settingsTopic: () => ({ nav: '', tab: TAB.settings }),
};

const isRouteName = (name: string): name is RouteName => Object.hasOwn(PLACES, name);

/**
 * The frame every page shows in, as every SyWare app has it: the navigation down the side, the page beside it, and
 * on a phone a top bar, the navigation in a drawer, and tabs under the page.
 */
export class ScAppShell extends ScElement {
  static readonly selector = 'sc-app-shell';

  // The page shown, to leave it when its project goes.
  private _route: ActiveRoute | undefined;
  // Whether this daemon can start with its computer: the palette offers the Startup topic then.
  private _startup = false;
  // Whether the swcode-base skills were asked about: the daemon answers often, the person once.
  private _baseOffered = false;
  // What the palette finds by itself, as it opened: the conversations the daemon finds join them.
  private _commands: PaletteCommand[] = [];

  private readonly _palette: { [Kind in keyof PaletteTargetMap]: (target: PaletteTargetMap[Kind]) => void } = {
    action: (target) => this.runAction(target.action),
    match: (target) => this.openMatch(target),
    project: (target) => navigate(projectPath(target.id)),
    session: (target) => navigate(sessionPath(target.id)),
    topic: (target) => navigate(settingsPath(target.id)),
  };

  private readonly _refs = this.useRefs((ref) => ({
    alert: ref(JlAlert),
    frame: ref(JlAppShell),
    nav: ref(ScNav),
    outlet: ref(JlPageOutlet),
    palette: ref(JlCommandPalette),
    projectDialog: ref(ScProjectDialog),
    tabs: ref(JlTabBar),
    topBrand: ref<HTMLAnchorElement>('.top-brand'),
    topSearch: ref(JlSearchButton),
  }));

  templateHtml(): Node {
    return (
      <JlAppShell narrowHeader>
        <div slot="header" class="top-bar">
          <a class="top-brand">
            <ScBrand />
          </a>
          <JlSearchButton label="Search" compact />
        </div>
        <ScNav slot="nav" />
        <div class="main">
          <JlAlert tone="danger" hidden />
          <JlPageOutlet />
        </div>
        <JlTabBar slot="tabs" more="More" label="swcode" />
        <JlToaster />
        <JlCommandPalette
          label="Search swcode"
          placeholder="Search sessions and what they say, projects, settings…"
        />
        <ScProjectDialog />
      </JlAppShell>
    );
  }

  templateCss(): string {
    return css`${BASE_STYLES}${styles}`;
  }

  onConnect(): void {
    this.mountPages();
    this.wireNavigation();
    this.wirePalette();
    this.wireProjects();
    this.watchDaemon();
    this.showWhatIsListed();
  }

  /** The router, and the page its address names. */
  private mountPages(): void {
    const outlet = this._refs.outlet;
    const router = new Router(ROUTES, { mode: 'hash', signal: this.killSignal });
    router.mountOn(outlet, { show: (page) => outlet.show(page) });
    window.addEventListener(HASH_CHANGE, () => this.follow(router.currentRoute), { signal: this.killSignal });
    this.follow(router.currentRoute);
  }

  /** The side nav: chat or code, the settings link closing a phone's drawer, and the mode the page is in. */
  private wireNavigation(): void {
    const nav = this._refs.nav;
    // Heard before the first page: it may switch the mode as it shows.
    this.onBus(workModeBus, 'changed', (mode) => this.renderMode(mode));
    this.renderMode(workMode());
    nav.events.register('mode', (mode) => {
      setWorkMode(mode);
      navigate(newPathOf(mode));
    });
    nav.events.register('settings', () => this._refs.frame.toggleNav(false));
    nav.events.register('search', () => this._refs.palette.show());
  }

  /** The command palette: what it finds by itself, and what the daemon finds in the conversations. */
  private wirePalette(): void {
    const palette = this._refs.palette;
    this._refs.topSearch.events.register('press', () => palette.show());
    palette.events.register('select', (id) => this.choose(paletteTarget(id)));
    palette.events.register('open', () => this.renderPalette());
    palette.events.register('search', (query) => palette.loading.set(searchSessions(query)));
    this.onBus(sessionSearchBus, 'found', (matches) => {
      palette.loading.set(false);
      palette.commands.set([...this._commands, ...matchCommands(matches, allSessions())]);
    });
    void this.offerStartup();
  }

  /** The project dialog the palette and the pages open, and what a new or removed project does. */
  private wireProjects(): void {
    this._refs.projectDialog.events.register('created', (project) => void this.openNewProject(project));
    this.onBus(projectListBus, 'open', () => this._refs.projectDialog.open());
    this.onBus(projectListBus, 'remove', (projectId) => void this.removeProject(projectId));
    this.onBus(projectListBus, 'changed', () => this._refs.nav.projects.set([...allProjects()]));
  }

  /** The daemon's state, and the sessions as the daemon lists them. */
  private watchDaemon(): void {
    this.onBus(daemonStatusBus, 'changed', (state) => {
      this.renderDaemon(state);
      offerUpdate(state);
      void this.offerSwcodeBaseSkills(state);
    });
    this.onBus(sessionListBus, 'changed', () => {
      this._refs.nav.sessions.set([...allSessions()]);
      // A session opened before the list came: its kind is known now.
      this.followMode(this._route);
    });
    this.onBus(seenBus, 'changed', () => this._refs.nav.seen.set({ ...seenSessions() }));
    this.onBus(sessionListBus, 'created', (session) => navigate(sessionPath(session.id)));
  }

  /**
   * Once the daemon answers, whether to offer the swcode-base skills: only on a machine where neither the home nor
   * any project has one, and only once per pack.
   */
  private async offerSwcodeBaseSkills(state: DaemonState): Promise<void> {
    // Not on a phone: the offer stays until it is answered, over the composer there, and installs on the computer.
    if (this._baseOffered || state.kind !== 'ready' || window.matchMedia(PHONE_QUERY).matches) {
      return;
    }
    this._baseOffered = true;
    const status = await readSwcodeBase();
    if (status !== undefined) {
      offerSwcodeBase(status);
    }
  }

  /** What the services already hold, so the frame shows it before the first answer comes. */
  private showWhatIsListed(): void {
    const nav = this._refs.nav;
    nav.projects.set([...allProjects()]);
    nav.sessions.set([...allSessions()]);
    nav.seen.set({ ...seenSessions() });
    this.renderDaemon(daemonState());
  }

  /** What the palette finds, as it opens: the sessions and projects of now; what was typed is looked for after. */
  private renderPalette(): void {
    const topics = SETTINGS_TOPICS.filter((topic) => this.offersTopic(topic));
    this._commands = paletteCommands(allProjects(), allSessions(), topics);
    this._refs.palette.commands.set(this._commands);
  }

  /**
   * The topics only some places have: the other computers in the desktop app, startup where the daemon can, updates
   * on a computer.
   */
  private offersTopic(topic: SettingsTopic): boolean {
    const offered: Partial<Record<SettingsTopic, () => boolean>> = {
      computers: inDesktopApp,
      startup: () => this._startup,
      updates: installsUpdates,
    };
    return offered[topic]?.() ?? true;
  }

  private async offerStartup(): Promise<void> {
    this._startup = (await loadStartup())?.available ?? false;
  }

  private choose(target: PaletteTarget | undefined): void {
    if (!target) {
      return;
    }
    this.goTo(target.kind, target);
  }

  private goTo<Kind extends keyof PaletteTargetMap>(kind: Kind, target: PaletteTargetMap[Kind]): void {
    this._palette[kind](target);
  }

  /** A session found by its conversation: it opens on the event that says the words, which the page marks. */
  private openMatch(target: PaletteTargetMap['match']): void {
    revealInSession(target.id, target.eventId);
    navigate(sessionPath(target.id));
  }

  private runAction(action: PaletteTargetMap['action']['action']): void {
    const actions: Record<typeof action, () => void> = {
      [PALETTE_ACTION.newChat]: () => navigate(NEW_CHAT_PATH),
      [PALETTE_ACTION.newProject]: () => askForProject(),
      [PALETTE_ACTION.newSession]: () => navigate(NEW_SESSION_PATH),
      [PALETTE_ACTION.phone]: () => navigate(settingsPath(PHONE_TOPIC)),
      [PALETTE_ACTION.theme]: () => this._refs.nav.switchTheme(),
    };
    actions[action]();
  }

  /** The daemon's state: a problem over every page, where a phone sees it. */
  private renderDaemon(state: DaemonState): void {
    const line = daemonLine(state);
    this._refs.alert.hidden = line.tone !== 'problem';
    this._refs.alert.heading.set(line.text);
    this._refs.alert.description.set(line.detail);
  }

  /** Removes a project once the person confirms; leaves its page, and its session when one of them is open. */
  /**
   * A project just added: a folder that was already there may bring instructions of its own (its AGENTS.md, its
   * skills), which the agent follows as the person's, so the person says first whether they trust it. Not now keeps
   * them from the agent until the project's Config says otherwise.
   */
  private async openNewProject(project: Project): Promise<void> {
    if (project.untrusted === true) {
      const trusted = await confirmDialog({
        cancelLabel: 'Not now',
        confirmLabel: 'Trust this folder',
        heading: `Trust ${project.name}?`,
        text: TRUST_QUESTION,
      });
      if (trusted) {
        setProjectTrust(project.id, true);
      }
    }
    navigate(newSessionPath(project.id));
  }

  private async removeProject(projectId: string): Promise<void> {
    const project = findProject(projectId);
    if (!project) {
      return;
    }
    const sessions = sessionsInProject(project, allSessions()).length;
    const confirmed = await confirmDialog({
      confirmLabel: 'Remove',
      heading: `Remove ${project.name}?`,
      text: removalConsequence(sessions),
      tone: 'danger',
    });
    if (!confirmed) {
      return;
    }
    daemon.send({ projectId, type: 'removeProject' });
    const shown = this._route;
    const onProject = shown?.name === 'project' && shown.params.id === projectId;
    const openSession = shown?.name === 'session' ? findSession(shown.params.id ?? '') : undefined;
    const inProject = openSession !== undefined && sessionInProject(project, openSession);
    if (onProject || inProject) {
      navigate(HOME_PATH);
    }
  }

  private follow(route: ActiveRoute): void {
    if (LEADING_ON.has(route.name)) {
      redirect(newPathOf(workMode()));
      return;
    }
    this._route = route;
    const place = this.placeOf(route);
    this._refs.nav.active.set(place.nav);
    this._refs.tabs.value.set(place.tab);
    this.followMode(route);
  }

  private placeOf(route: ActiveRoute): Place {
    return isRouteName(route.name) ? PLACES[route.name](route.params) : { nav: '', tab: '' };
  }

  /** A page of a mode switches the app to it: a chat's shows the chats, a project's the projects. */
  private followMode(route: ActiveRoute | undefined): void {
    const mode = route ? this.placeOf(route).mode : undefined;
    if (mode === undefined) {
      return;
    }
    setWorkMode(mode);
  }

  /** The mode's sessions in the navigation, and its new one under swcode's name and a phone's New tab. */
  private renderMode(mode: SessionKind): void {
    this._refs.nav.mode.set(mode);
    this._refs.tabs.items.set(tabsFor(mode));
    this._refs.topBrand.href = hrefOf(newPathOf(mode));
  }
}

ScAppShell.register();
