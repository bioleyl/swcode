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
import { PALETTE_ACTION, paletteCommands, paletteTarget } from '../lib/palette';
import { removalConsequence } from '../lib/project-groups';
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
import { allProjects, askForProject, findProject, projectListBus } from '../services/project-list';
import { seenBus, seenSessions } from '../services/seen-sessions';
import { allSessions, findSession, sessionListBus } from '../services/session-list';
import { loadStartup } from '../services/startup';
import { setWorkMode, workMode, workModeBus } from '../services/work-mode';
import { BASE_STYLES } from '../styles/shared';
import styles from './sc-app-shell.css?inline';

import type { ActiveRoute } from '@jadis/core';
import type { TabBarItem } from '@syware/jlib';
import type { SessionKind } from '../../../shared/protocol/session.schema';
import type { SettingsTopic } from '../lib/settings-topics';
import type { RouteName } from '../routes';
import type { DaemonState } from '../types/daemon.types';
import type { PaletteTarget, PaletteTargetMap } from '../types/palette.types';

const HASH_CHANGE = 'hashchange';

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

  private readonly _palette: { [Kind in keyof PaletteTargetMap]: (target: PaletteTargetMap[Kind]) => void } = {
    action: (target) => this.runAction(target.action),
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
        <JlCommandPalette label="Search swcode" placeholder="Search sessions, projects, settings…" />
        <ScProjectDialog />
      </JlAppShell>
    );
  }

  templateCss(): string {
    return css`${BASE_STYLES}${styles}`;
  }

  onConnect(): void {
    const nav = this._refs.nav;
    const outlet = this._refs.outlet;
    // Heard before the first page: it may switch the mode as it shows.
    this.onBus(workModeBus, 'changed', (mode) => this.renderMode(mode));
    this.renderMode(workMode());
    nav.events.register('mode', (mode) => {
      setWorkMode(mode);
      navigate(newPathOf(mode));
    });
    const router = new Router(ROUTES, { mode: 'hash', signal: this.killSignal });
    router.mountOn(outlet, { show: (page) => outlet.show(page) });
    window.addEventListener(HASH_CHANGE, () => this.follow(router.currentRoute), { signal: this.killSignal });
    this.follow(router.currentRoute);
    // The settings' link, at the nav's foot, closes a phone's drawer as the nav's own links do.
    nav.events.register('settings', () => this._refs.frame.toggleNav(false));
    nav.events.register('search', () => this._refs.palette.show());
    this._refs.topSearch.events.register('press', () => this._refs.palette.show());
    this._refs.palette.events.register('select', (id) => this.choose(paletteTarget(id)));
    this._refs.palette.events.register('open', () => this.renderPalette());
    void this.offerStartup();
    this._refs.projectDialog.events.register('created', (project) => navigate(newSessionPath(project.id)));
    this.onBus(projectListBus, 'open', () => this._refs.projectDialog.open());
    this.onBus(projectListBus, 'remove', (projectId) => void this.removeProject(projectId));
    this.onBus(projectListBus, 'changed', () => nav.projects.set([...allProjects()]));
    this.onBus(sessionListBus, 'changed', () => {
      nav.sessions.set([...allSessions()]);
      // A session opened before the list came: its kind is known now.
      this.followMode(this._route);
    });
    this.onBus(seenBus, 'changed', () => nav.seen.set({ ...seenSessions() }));
    this.onBus(sessionListBus, 'created', (session) => navigate(sessionPath(session.id)));
    this.onBus(daemonStatusBus, 'changed', (state) => this.renderDaemon(state));
    nav.projects.set([...allProjects()]);
    nav.sessions.set([...allSessions()]);
    nav.seen.set({ ...seenSessions() });
    this.renderDaemon(daemonState());
  }

  /** What the palette finds, as it opens: the sessions and projects of now. */
  private renderPalette(): void {
    const topics = SETTINGS_TOPICS.filter((topic) => this.offersTopic(topic));
    this._refs.palette.commands.set(paletteCommands(allProjects(), allSessions(), topics));
  }

  /** The topics only some places have: the other computers in the desktop app, startup where the daemon can. */
  private offersTopic(topic: SettingsTopic): boolean {
    if (topic === 'computers') {
      return inDesktopApp();
    }
    return topic !== 'startup' || this._startup;
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

  /** The daemon's state: the navigation's computer, and a problem over every page, where a phone sees it. */
  private renderDaemon(state: DaemonState): void {
    this._refs.nav.daemon.set(state);
    const line = daemonLine(state);
    this._refs.alert.hidden = line.tone !== 'problem';
    this._refs.alert.heading.set(line.text);
    this._refs.alert.description.set(line.detail);
  }

  /** Removes a project once the person confirms; leaves its page, and its session when one of them is open. */
  private async removeProject(projectId: string): Promise<void> {
    const project = findProject(projectId);
    if (!project) {
      return;
    }
    const sessions = allSessions().filter((session) => session.workspace === project.path).length;
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
    const inProject = shown?.name === 'session' && findSession(shown.params.id ?? '')?.workspace === project.path;
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
