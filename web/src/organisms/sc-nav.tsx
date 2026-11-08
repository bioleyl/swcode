import { css } from '@jadis/core';
import { JlButton, JlIcon, JlSearchButton, JlSegmented, JlSideNav, jlText, registerIcons } from '@syware/jlib';
import { Laptop, PanelLeftClose, PanelLeftOpen, Server } from 'lucide';

import { ScBrand } from '../atoms/sc-brand';
import { ScElement } from '../base/sc-element';
import { HOST_ICONS } from '../lib/hosts';
import { groupByProject } from '../lib/project-groups';
import { sessionsOf } from '../lib/session-kinds';
import { LIT_ICONS, litIcon, projectLight, SESSION_ICON, sessionLight } from '../lib/session-lights';
import { waitingCount } from '../lib/session-status';
import { ScComputerSwitch } from '../molecules/sc-computer-switch';
import { keepNavFolded, navFolded } from '../services/nav-fold';
import {
  hrefOf,
  NEW_CHAT_PATH,
  NEW_SESSION_PATH,
  newPathOf,
  newSessionPath,
  projectPath,
  SETTINGS_PATH,
  sessionPath,
} from '../services/navigation';
import { currentTheme, toggleTheme } from '../services/theme';
import { BASE_STYLES } from '../styles/shared';
import styles from './sc-nav.css?inline';
import rowStyles from './sc-nav-rows.css?inline';
import scrollStyles from './sc-nav-scroll.css?inline';

import type { SegmentedOption, SideNavItem, SideNavSection } from '@syware/jlib';
import type { Project } from '../../../shared/protocol/project.schema';
import type { SessionKind, SessionSummary } from '../../../shared/protocol/session.schema';
import type { Theme } from '../services/theme';
import type { ProjectGroup } from '../types/conversation.types';
import type { DaemonState } from '../types/daemon.types';
import type { NavLists, ProjectStart } from '../types/nav.types';

registerIcons({
  laptop: Laptop,
  'panel-left-close': PanelLeftClose,
  'panel-left-open': PanelLeftOpen,
  server: Server,
  ...LIT_ICONS,
});

/** The navigation's own items; a project's and a session's ids carry theirs (`project:…`, `session:…`). */
export const NAV_ID = { newChat: 'new-chat', newSession: 'new' } as const;
const PROJECT_PREFIX = 'project:';
const SESSION_PREFIX = 'session:';
// What jlib's side nav says on itself while it is a rail.
const RAIL = 'rail';
// What jlib's app shell says on its nav while it is a phone's drawer: the side nav inside shows whole then.
const IN_DRAWER = 'in-drawer';
const COLLAPSE_ICON = 'panel-left-close';
const EXPAND_ICON = 'panel-left-open';
const THEME_ICONS: Readonly<Record<Theme, string>> = { dark: 'sun', light: 'moon' };
const PLUS_SIZE = 14;
// What marks swcode's own nodes in jlib's side nav: its styles, and the + of each project row.
const ROW_STYLES_CLASS = 'sc-rows';
const NEW_SESSION_CLASS = 'sc-new-session';
// What marks swcode's fix of jlib's side nav layout, its header and foot in place (see `sc-nav-scroll.css`).
const SCROLL_STYLES_CLASS = 'sc-nav-scroll';
// A project's row in jlib's side nav, not one of its sessions'.
const PROJECT_ROWS = `.item[data-depth="0"][data-id^="${PROJECT_PREFIX}"]`;
const SETTINGS = 'Settings';
// Chat or code, at the nav's foot: the sessions the nav lists, and the new one it starts.
const WORK_MODES: SegmentedOption<SessionKind>[] = [
  { label: 'Chat', value: 'chat' },
  { label: 'Code', value: 'code' },
];

export const projectNavId = (id: string): string => `${PROJECT_PREFIX}${id}`;
export const sessionNavId = (id: string): string => `${SESSION_PREFIX}${id}`;

/** A session, its bubble filled while it works, waits for the person, or is done and not seen yet. */
const sessionItem = (session: SessionSummary, seen: Readonly<Record<string, string>>): SideNavItem => ({
  href: hrefOf(sessionPath(session.id)),
  icon: litIcon(SESSION_ICON, sessionLight(session, seen[session.id])),
  id: sessionNavId(session.id),
  label: session.title,
});

/**
 * A project and its sessions, its icon filled as the one of them that most needs the person, the count of those that
 * wait for them beside its name.
 */
const projectItem = (group: ProjectGroup, seen: Readonly<Record<string, string>>): SideNavItem => ({
  badge: waitingCount(group.sessions) || undefined,
  children: group.sessions.map((session) => sessionItem(session, seen)),
  ...(group.projectId === undefined ? {} : { href: hrefOf(projectPath(group.projectId)) }),
  icon: litIcon(
    HOST_ICONS[group.host.kind],
    projectLight(group.sessions.map((session) => sessionLight(session, seen[session.id])))
  ),
  id: projectNavId(group.projectId ?? group.path),
  label: group.name,
});

/** Where the + of each project row leads, by the row's id: a new session there. A folder without a project has none. */
const projectStarts = (sections: readonly SideNavSection[]): Map<string, ProjectStart> =>
  new Map(
    sections
      .flatMap((section) => section.items)
      .filter((item) => item.id.startsWith(PROJECT_PREFIX) && item.href !== undefined)
      .map((item) => [
        item.id,
        { href: hrefOf(newSessionPath(item.id.slice(PROJECT_PREFIX.length))), name: item.label },
      ])
  );

/** swcode's styles for what it adds to jlib's side nav, once. */
const addRowStyles = (root: ShadowRoot): void => {
  if (root.querySelector(`style.${ROW_STYLES_CLASS}`)) {
    return;
  }
  root.append(<style class={ROW_STYLES_CLASS}>{rowStyles}</style>);
};

/** swcode's fix for jlib's side nav scrolling as one column, its header and foot leaving with the sessions, once. */
const addScrollStyles = (root: ShadowRoot): void => {
  if (root.querySelector(`style.${SCROLL_STYLES_CLASS}`)) {
    return;
  }
  root.append(<style class={SCROLL_STYLES_CLASS}>{scrollStyles}</style>);
};

/** What the nav lists in each mode: a new chat and the chats, or a new session and the projects with theirs. */
const SECTIONS: Readonly<Record<SessionKind, (lists: NavLists) => SideNavSection[]>> = {
  chat: ({ sessions, seen }) => [
    { items: [{ href: hrefOf(NEW_CHAT_PATH), icon: 'plus', id: NAV_ID.newChat, label: 'New chat' }] },
    { items: sessionsOf('chat', sessions).map((session) => sessionItem(session, seen)), label: 'Chats' },
  ],
  code: ({ projects, sessions, seen }) => [
    { items: [{ href: hrefOf(NEW_SESSION_PATH), icon: 'plus', id: NAV_ID.newSession, label: 'New session' }] },
    {
      items: groupByProject(projects, sessionsOf('code', sessions)).map((group) => projectItem(group, seen)),
      label: 'Projects',
    },
  ],
};

/**
 * The side of every page: swcode and where it works; in Code, a new session and the projects with their sessions
 * (what needs the person counted beside each), in Chat a new chat and the chats; at the foot the fold button, chat or
 * code, the settings and the theme. It folds into a rail, as the person left it last on this device, its foot keeping
 * the fold button alone.
 */
export class ScNav extends ScElement {
  static readonly selector = 'sc-nav';

  /** `settings`: its link pressed, which a phone's drawer closes for; `mode`: chat or code picked at its foot. */
  readonly events = this.useEvents<{ mode: SessionKind; search: undefined; settings: undefined }>();

  /** Chat or code: the sessions listed, and where swcode's name leads, the new one of that kind. */
  readonly mode = this.useChange<SessionKind>(
    'code',
    (mode) => {
      this._refs.modes.value.set(mode);
      this._refs.brand.href = hrefOf(newPathOf(mode));
      this.renderSections();
    },
    { immediate: true }
  );

  readonly projects = this.useChange<Project[]>([], () => this.renderSections());
  readonly sessions = this.useChange<SessionSummary[]>([], () => this.renderSections());
  /** When the person last saw each session, by its last activity then. */
  readonly seen = this.useChange<Record<string, string>>({}, () => this.renderSections());
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

  // The + of each project row: where it leads, by the row's id.
  private _starts = new Map<string, ProjectStart>();

  private readonly _refs = this.useRefs((ref) => ({
    brand: ref<HTMLAnchorElement>('.brand'),
    computer: ref(ScComputerSwitch),
    fold: ref<JlButton>('.fold'),
    modes: ref<JlSegmented<SessionKind>>('.mode'),
    nav: ref(JlSideNav),
    search: ref(JlSearchButton),
    settings: ref<JlButton>('.settings'),
    theme: ref<JlButton>('.theme'),
  }));

  templateHtml(): Node {
    return (
      <JlSideNav label="swcode" collapsed={navFolded()}>
        <div slot="header" class="header">
          <a class="brand">
            <ScBrand />
          </a>
          <JlSearchButton label="Search" />
          <ScComputerSwitch />
        </div>
        <div slot="footer" class="footer">
          <JlButton class="fold" variant="ghost" size="sm" />
          <JlSegmented<SessionKind> class="mode" size="sm" block label="Work in" options={WORK_MODES} />
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
    this._refs.modes.events.register('change', (mode) => this.events.emit('mode', mode));
    this.followRail();
    this._refs.search.events.register('press', () => this.events.emit('search'));
    this._refs.settings.events.register('press', () => this.events.emit('settings'));
    this._refs.theme.events.register('press', () => this.switchTheme());
    this.renderTheme(currentTheme());
    this.renderSections();
    this.patchSideNav();
  }

  /**
   * Folds the side nav into its rail, or unfolds it, and keeps it so on this device. swcode's own button, on the
   * footer's row beside the settings and the theme, where jlib's cannot go.
   */
  private fold(): void {
    const folded = !this._refs.nav.collapsed.get();
    this._refs.nav.collapsed.set(folded);
    keepNavFolded(folded);
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
    const lists = { projects: this.projects.get(), seen: this.seen.get(), sessions: this.sessions.get() };
    const sections = SECTIONS[this.mode.get()](lists);
    this._starts = projectStarts(sections);
    this._refs.nav.sections.set(sections);
  }

  /**
   * What swcode puts into jlib's side nav itself: its header and foot kept in place, the sections scrolling between
   * them (`sc-nav-scroll.css`), and the + that starts a session in a project, swapped in on its row each time jlib
   * draws the rows again (new sessions, folding, the phone's drawer), since jlib's items take no button of the app's
   * on a row (0.11.1). Both go once jlib has them.
   */
  private patchSideNav(): void {
    const root = this._refs.nav.shadowRoot;
    if (!root) {
      return;
    }
    addScrollStyles(root);
    const observer = new MutationObserver(() => this.addProjectStarts(root));
    observer.observe(root, { childList: true, subtree: true });
    this.killSignal.addEventListener('abort', () => observer.disconnect(), { once: true });
    this.addProjectStarts(root);
  }

  /** The + of each project row that has none yet, and the styles that place it. */
  private addProjectStarts(root: ShadowRoot): void {
    addRowStyles(root);
    for (const row of root.querySelectorAll<HTMLElement>(PROJECT_ROWS)) {
      this.addProjectStart(row);
    }
  }

  /** The + beside a project's row, in its entry as jlib's chevron is: once, and for a project only. */
  private addProjectStart(row: HTMLElement): void {
    const start = this._starts.get(row.dataset.id ?? '');
    const entry = row.parentElement;
    if (!start || !entry || entry.querySelector(`:scope > .${NEW_SESSION_CLASS}`)) {
      return;
    }
    entry.append(this.startLink(start));
  }

  /** A link, so the phone's drawer closes on it as on the nav's own links, and a middle click opens a tab. */
  private startLink(start: ProjectStart): Node {
    const label = `New session in ${start.name}`;
    return (
      <a class={NEW_SESSION_CLASS} href={start.href} title={label} aria-label={label}>
        <JlIcon name="plus" size={PLUS_SIZE} />
      </a>
    );
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
