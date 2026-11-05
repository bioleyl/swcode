import type { IconName } from './icons';

/** The settings page's topics, in the order its menu lists them. */
export const SETTINGS_TOPICS = [
  'models',
  'thinking',
  'projects',
  'pictures',
  'network',
  'phone',
  'startup',
  'computers',
  'skills',
  'plugins',
] as const;
export type SettingsTopic = (typeof SETTINGS_TOPICS)[number];

export const DEFAULT_SETTINGS_TOPIC: SettingsTopic = 'models';

/** Pairing a phone, and the phones paired: the palette leads there too. */
export const PHONE_TOPIC: SettingsTopic = 'phone';

/** How each topic shows in the menu: a new topic does not compile until it is here. */
export const SETTINGS_TOPIC_LABELS: Readonly<Record<SettingsTopic, { label: string; icon: IconName }>> = {
  computers: { icon: 'laptop', label: 'Computers' },
  models: { icon: 'cpu', label: 'Models' },
  network: { icon: 'network', label: 'Network' },
  phone: { icon: 'smartphone', label: 'Phone' },
  pictures: { icon: 'image', label: 'Pictures' },
  plugins: { icon: 'puzzle', label: 'Plugins' },
  projects: { icon: 'folder', label: 'Projects' },
  skills: { icon: 'book-open', label: 'Skills' },
  startup: { icon: 'power', label: 'Startup' },
  thinking: { icon: 'brain', label: 'Thinking' },
};

export const isSettingsTopic = (value: string | null | undefined): value is SettingsTopic =>
  SETTINGS_TOPICS.some((topic) => topic === value);
