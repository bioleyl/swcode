import type { IconName } from './icons';

/** A project page's tabs, in the order it shows them. */
export const PROJECT_TABS = ['sessions', 'config', 'skills'] as const;
export type ProjectTab = (typeof PROJECT_TABS)[number];

export const DEFAULT_PROJECT_TAB: ProjectTab = 'sessions';

/** How each tab shows: a new tab does not compile until it is here. */
export const PROJECT_TAB_LABELS: Readonly<Record<ProjectTab, { label: string; icon: IconName }>> = {
  config: { icon: 'sliders-horizontal', label: 'Config' },
  sessions: { icon: 'terminal', label: 'Sessions' },
  skills: { icon: 'book-open', label: 'Skills' },
};

export const isProjectTab = (value: string | null | undefined): value is ProjectTab =>
  PROJECT_TABS.some((tab) => tab === value);
