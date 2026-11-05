import { css } from '@jadis/core';
import { JlButton, JlSearchButton, JlSegmented, JlSideNav, jlText, registerIcons } from '@syware/jlib';
import { Laptop, PanelLeftClose, PanelLeftOpen, Server } from 'lucide';

import { ScBrand } from '../atoms/sc-brand';
import { ScElement } from '../base/sc-element';
import { HOST_ICONS } from '../lib/hosts';
import { groupByProject } from '../lib/project-groups';
import { SESSION_STATUS_ICONS, waitingCount } from '../lib/session-status';
import { ScComputerSwitch } from '../molecules/sc-computer-switch';
import {
  HOME_PATH,
  hrefOf,
  NEW_SESSION_PATH,
  projectPath,
  SETTINGS_PATH,
  sessionPath,
} from '../services/navigation';
import { currentTheme, toggleTheme } from '../services/theme';
import { BASE_STYLES } from '../styles/shared';
import styles from './sc-nav.css?inline';

import type { SegmentedOption, SideNavItem, SideNavSection } from '@syware/jlib';
import type { Project } from '../../../shared/protocol/project.schema';
import type { SessionSummary } from '../../../shared/protocol/session.schema';
import type { Theme } from '../services/theme';
import type { ProjectGroup } from '../types/conversation.types';
import type { DaemonState } from '../types/daemon.types';
import type { WorkMode } from '../types/nav.types';

registerIcons({
  laptop: Laptop,
  'panel-left-close': PanelLeftClose,
  'panel-left-open': PanelLeftOpen,
  server: Server,
});

/** The navigation's own item; a project's and a session's ids carry theirs (`project:…`, `session:…`). */
export const NAV_ID = { newSession: 'new' } as const;
const PROJECT_PREFIX = 'project:';
const SESSION_PREFIX = 'session:';
const FOLDED_KEY = 'swcode.nav.folded';
// What jlib's side nav says on itself while it is a rail.
const RAIL = 'rail';
// What jlib's app shell says on its nav while it is a phone's drawer: the side nav inside shows whole then.
const IN_DRAWER = 'in-drawer';
const COLLAPSE_ICON = 'panel-left-close';
const EXPAND_ICON = 'panel-left-open';
const THEME_ICONS: Readonly<Record<Theme, string>> = { dark: 'sun', light: 'moon' };
const SETTINGS = 'Settings';
// Chat or code, at the nav's foot: it chooses nothing yet.
const WORK_MODES: SegmentedOption<WorkMode>[] = [
  { label: 'Chat', value: 'chat' },
  { label: 'Code', value: 'code' },
];
const DEFAULT_WORK_MODE: WorkMode = 'code';

export const projectNavId = (id: string): string => `${PROJECT_PREFIX}${id}`;
export const sessionNavId = (id: string): string => `${SESSION_PREFIX}${id}`;

const storedFold = (): boolean => {
  try {
    return localStorage.getItem(FOLDED_KEY) === 'true';
  } catch {
    return false;
  }
};

const keepFold = (folded: boolean): void => {
  try {
    localStorage.setItem(FOLDED_KEY, String(folded));
  } catch {
    // Without storage, the side nav opens unfolded next time.
  }
};

const sessionItem = (session: SessionSummary): SideNavItem => ({
  href: hrefOf(sessionPath(session.id)),
  icon: SESSION_STATUS_ICONS[session.status],
  id: sessionNavId(session.id),
  label: session.title,
});

/** A project and its sessions, the count of those that need the person beside its name. */
const projectItem = (group: ProjectGroup): SideNavItem => ({
  badge: waitingCount(group.sessions) || undefined,
  children: group.sessions.map(sessionItem),
  ...(group.projectId === undefined ? {} : { href: hrefOf(projectPath(group.projectId)) }),
  icon: HOST_ICONS[group.host.kind],
  id: projectNavId(group.projectId ?? group.path),
  label: group.name,
});

const sectionsOf = (projects: readonly Project[], sessions: readonly SessionSummary[]): SideNavSection[] => [
  { items: [{ href: hrefOf(NEW_SESSION_PATH), icon: 'plus', id: NAV_ID.newSession, label: 'New session' }] },
  { items: groupByProject(projects, sessions).map(projectItem), label: 'Projects' },
];

/**
 * The side of every page: swcode and where it works, a new session, the projects with their sessions (what needs
 * the person counted beside each), and at the foot the fold button, chat or code (which chooses nothing yet), the
 * settings and the theme. It folds into a rail, as the person left it last on this device, its foot keeping the fold
 * button alone.
 */
export class ScNav extends ScElement {
  static readonly selector = 'sc-nav';

  /** `settings`: its link pressed, which a phone's drawer closes for. */
  readonly events = this.useEvents<{ search: undefined; settings: undefined }>();

  readonly projects = this.useChange<Project[]>([], () => this.renderSections());
  readonly sessions = this.useChange<SessionSummary[]>([], () => this.renderSections());
  /** The item of the page shown: `new`, `settings`, `project:…`, `session:…`. */
  readonly active = this.useChange('', (id) => this._refs.nav.value.set(id));
  /** The daemon's state: the computer it runs on, in a browser. */
  readonly daemon = this.useChange<DaemonState>({ kind: 'unknown' }, (state) => this.renderComputer(state), {
    immediate: true,
  });

  // The shell marks what it holds as its nav, this element: the side nav inside learns it from here.
  readonly drawer = this.useAttributes({
    [IN_DRAWER]: (value) => {
      this._refs.nav.inDrawer.set(value !== null);
      this._refs.fold.hidden = value !== null;
    },
  });

  private readonly _refs = this.useRefs((ref) => ({
    computer: ref(ScComputerSwitch),
    fold: ref<JlButton>('.fold'),
    nav: ref(JlSideNav),
    search: ref(JlSearchButton),
    settings: ref<JlButton>('.settings'),
    theme: ref<JlButton>('.theme'),
  }));

  templateHtml(): Node {
    return (
      <JlSideNav label="swcode" collapsed={storedFold()}>
        <div slot="header" class="header">
          <a class="brand" href={hrefOf(HOME_PATH)}>
            <ScBrand />
          </a>
          <JlSearchButton label="Search" />
          <ScComputerSwitch />
        </div>
        <div slot="footer" class="footer">
          <JlButton class="fold" variant="ghost" size="sm" />
          <JlSegmented<WorkMode>
            class="mode"
            size="sm"
            block
            label="Work in"
            options={WORK_MODES}
            value={DEFAULT_WORK_MODE}
          />
          <JlButton
            class="settings"
            variant="ghost"
            size="sm"
            icon="settings"
            hint={SETTINGS}
            href={hrefOf(SETTINGS_PATH)}
          />
          <JlButton class="theme" variant="ghost" size="sm" />
        </div>
      </JlSideNav>
    );
  }

  templateCss(): string {
    return css`${BASE_STYLES}${styles}`;
  }

  onConnect(): void {
    this._refs.fold.events.register('press', () => this.fold());
    this.followRail();
    this._refs.search.events.register('press', () => this.events.emit('search'));
    this._refs.settings.events.register('press', () => this.events.emit('settings'));
    this._refs.theme.events.register('press', () => this.switchTheme());
    this.renderTheme(currentTheme());
    this.renderSections();
  }

  /**
   * Folds the side nav into its rail, or unfolds it, and keeps it so on this device. swcode's own button, on the
   * footer's row beside the settings and the theme, where jlib's cannot go.
   */
  private fold(): void {
    const folded = !this._refs.nav.collapsed.get();
    this._refs.nav.collapsed.set(folded);
    keepFold(folded);
  }

  /**
   * The search shrinks to its icon while the side nav is a rail (folded, and not a phone's drawer, which shows it
   * whole), and the fold button turns to unfold it. jlib says which on the side nav's `rail` attribute.
   */
  private followRail(): void {
    const nav = this._refs.nav;
    const render = (): void => {
      const rail = nav.hasAttribute(RAIL);
      this._refs.search.compact.set(rail);
      this._refs.fold.icon.set(rail ? EXPAND_ICON : COLLAPSE_ICON);
      this._refs.fold.hint.set(jlText(rail ? 'expandMenu' : 'collapseMenu'));
    };
    const observer = new MutationObserver(render);
    observer.observe(nav, { attributeFilter: [RAIL], attributes: true });
    this.killSignal.addEventListener('abort', () => observer.disconnect(), { once: true });
    render();
  }

  private renderSections(): void {
    this._refs.nav.sections.set(sectionsOf(this.projects.get(), this.sessions.get()));
  }

  /** The other theme, and the button that offers it back. */
  switchTheme(): void {
    this.renderTheme(toggleTheme());
  }

  /** The button offers the other theme. */
  private renderTheme(theme: Theme): void {
    this._refs.theme.icon.set(THEME_ICONS[theme]);
    this._refs.theme.hint.set(theme === 'dark' ? 'Light theme' : 'Dark theme');
  }

  private renderComputer(state: DaemonState): void {
    this._refs.computer.computer.set(state.kind === 'ready' ? (state.status.computer ?? '') : '');
  }
}

ScNav.register();
