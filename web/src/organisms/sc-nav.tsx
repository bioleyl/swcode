import { css } from '@jadis/core';
import { JlButton, JlSearchButton, JlSideNav, jlText, registerIcons } from '@syware/jlib';
import { BookOpen, FolderPlus, Laptop, PanelLeftClose, PanelLeftOpen, Server, Smartphone } from 'lucide';

import { ScBrand } from '../atoms/sc-brand';
import { ScElement } from '../base/sc-element';
import { daemonLine } from '../lib/daemon-lines';
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
  settingsPath,
} from '../services/navigation';
import { currentTheme, toggleTheme } from '../services/theme';
import { BASE_STYLES } from '../styles/shared';
import styles from './sc-nav.css?inline';

import type { SideNavItem, SideNavSection } from '@syware/jlib';
import type { Project } from '../../../shared/protocol/project.schema';
import type { SessionSummary } from '../../../shared/protocol/session.schema';
import type { Theme } from '../services/theme';
import type { ProjectGroup } from '../types/conversation.types';
import type { DaemonState } from '../types/daemon.types';

registerIcons({
  'book-open': BookOpen,
  'folder-plus': FolderPlus,
  laptop: Laptop,
  'panel-left-close': PanelLeftClose,
  'panel-left-open': PanelLeftOpen,
  server: Server,
  smartphone: Smartphone,
});

/** The navigation's own items; a project's and a session's ids carry theirs (`project:…`, `session:…`). */
export const NAV_ID = {
  newProject: 'new-project',
  newSession: 'new',
  phone: 'phone',
  settings: 'settings',
  skills: 'skills',
} as const;
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
  {
    items: [
      ...groupByProject(projects, sessions).map(projectItem),
      { icon: 'folder-plus', id: NAV_ID.newProject, label: 'New project' },
    ],
    label: 'Projects',
  },
  {
    items: [
      { icon: 'smartphone', id: NAV_ID.phone, label: 'Phone' },
      { href: hrefOf(settingsPath(NAV_ID.skills)), icon: 'book-open', id: NAV_ID.skills, label: 'Skills' },
      { href: hrefOf(SETTINGS_PATH), icon: 'settings', id: NAV_ID.settings, label: 'Settings' },
    ],
  },
];

/**
 * The side of every page: swcode and where it works, a new session, the projects with their sessions (what needs
 * the person counted beside each), the phone, the skills and the settings, and whether the daemon and llama-server
 * answer. It folds into a rail, as the person left it last on this device.
 */
export class ScNav extends ScElement {
  static readonly selector = 'sc-nav';

  readonly events = this.useEvents<{ newProject: undefined; phone: undefined; search: undefined }>();

  readonly projects = this.useChange<Project[]>([], () => this.renderSections());
  readonly sessions = this.useChange<SessionSummary[]>([], () => this.renderSections());
  /** The item of the page shown: `new`, `settings`, `project:…`, `session:…`. */
  readonly active = this.useChange('', (id) => this._refs.nav.value.set(id));
  readonly daemon = this.useChange<DaemonState>({ kind: 'unknown' }, (state) => this.renderDaemon(state), {
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
    detail: ref('.daemon-detail'),
    fold: ref<JlButton>('.fold'),
    nav: ref(JlSideNav),
    search: ref(JlSearchButton),
    state: ref('.daemon-state'),
    status: ref('.status'),
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
          <div class="status">
            <span class="dot" aria-hidden="true" />
            <span class="daemon-text">
              <span class="daemon-state sr-only" />
              <span class="daemon-detail" />
            </span>
            <JlButton class="theme" variant="ghost" size="sm" />
          </div>
          <JlButton class="fold" variant="ghost" size="sm" block />
        </div>
      </JlSideNav>
    );
  }

  templateCss(): string {
    return css`${BASE_STYLES}${styles}`;
  }

  onConnect(): void {
    this._refs.nav.events.register('select', (id) => this.choose(id));
    this._refs.fold.events.register('press', () => this.fold());
    this.followRail();
    this._refs.search.events.register('press', () => this.events.emit('search'));
    this._refs.theme.events.register('press', () => this.switchTheme());
    this.renderTheme(currentTheme());
    this.renderSections();
  }

  /**
   * Folds the side nav into its rail, or unfolds it, and keeps it so on this device. swcode's own button, the whole
   * width of the nav: jlib's is as narrow as its label, and nothing outside its shadow root can widen it. It goes
   * once jlib's can (https://github.com/bioleyl/jlib/issues/2).
   */
  private fold(): void {
    const folded = !this._refs.nav.collapsed.get();
    this._refs.nav.collapsed.set(folded);
    keepFold(folded);
  }

  /**
   * The search and the fold button shrink to their icon while the side nav is a rail: folded, and not a phone's
   * drawer, which shows it whole. jlib says which on the side nav's `rail` attribute.
   */
  private followRail(): void {
    const nav = this._refs.nav;
    const render = (): void => {
      const rail = nav.hasAttribute(RAIL);
      this._refs.search.compact.set(rail);
      this._refs.fold.icon.set(rail ? EXPAND_ICON : COLLAPSE_ICON);
      this._refs.fold.label.set(rail ? '' : jlText('collapseMenu'));
      this._refs.fold.hint.set(rail ? jlText('expandMenu') : '');
    };
    const observer = new MutationObserver(render);
    observer.observe(nav, { attributeFilter: [RAIL], attributes: true });
    this.killSignal.addEventListener('abort', () => observer.disconnect(), { once: true });
    render();
  }

  private choose(id: string): void {
    if (id === NAV_ID.phone) {
      this.events.emit('phone');
      return;
    }
    if (id === NAV_ID.newProject) {
      this.events.emit('newProject');
    }
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

  private renderDaemon(state: DaemonState): void {
    const line = daemonLine(state);
    // The dot and the address say it; the words are for screen readers, and the tooltip.
    this._refs.state.textContent = line.text;
    this._refs.detail.textContent = line.detail;
    this._refs.status.title = line.detail === '' ? line.text : `${line.text}: ${line.detail}`;
    this._refs.status.dataset.tone = line.tone;
    this._refs.computer.computer.set(state.kind === 'ready' ? (state.status.computer ?? '') : '');
  }
}

ScNav.register();
