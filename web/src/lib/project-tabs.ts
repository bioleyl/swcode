import type { IconName } from './icons';

/** A project page's tabs, in the order it shows them. */
export const PROJECT_TABS = ['sessions', 'todos', 'config', 'skills'] as const;
export type ProjectTab = (typeof PROJECT_TABS)[number];

export const DEFAULT_PROJECT_TAB: ProjectTab = 'sessions';

/** How each tab shows: a new tab does not compile until it is here. */
export const PROJECT_TAB_LABELS: Readonly<Record<ProjectTab, { label: string; icon: IconName }>> = {
  config: { icon: 'sliders-horizontal', label: 'Config' },
  sessions: { icon: 'terminal', label: 'Sessions' },
  skills: { icon: 'book-open', label: 'Skills' },
  todos: { icon: 'list-checks', label: 'Todos' },
};

export const isProjectTab = (value: string | null | undefined): value is ProjectTab =>
  PROJECT_TABS.some((tab) => tab === value);

/** The Config tab's own tabs, in the order it shows them: the defaults, what the project may do, its schedules. */
export const CONFIG_TABS = ['defaults', 'permissions', 'schedules'] as const;
export type ConfigTab = (typeof CONFIG_TABS)[number];

/** How each of Config's tabs shows: a new one does not compile until it is here. */
export const CONFIG_TAB_LABELS: Readonly<Record<ConfigTab, { label: string; icon: IconName }>> = {
  defaults: { icon: 'sliders-horizontal', label: 'Defaults' },
  permissions: { icon: 'shield', label: 'Permissions' },
  schedules: { icon: 'refresh', label: 'Schedules' },
};
