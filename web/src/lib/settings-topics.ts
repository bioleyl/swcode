import type { IconName } from './icons';

/** The settings page's topics, in the order its menu lists them. */
export const SETTINGS_TOPICS = [
  'models',
  'thinking',
  'projects',
  'ssh',
  'mcp',
  'pictures',
  'network',
  'startup',
  'computers',
  'updates',
] as const;
export type SettingsTopic = (typeof SETTINGS_TOPICS)[number];

export const DEFAULT_SETTINGS_TOPIC: SettingsTopic = 'models';

/** The MCP servers whose tools the agent has: read while their topic is shown. */
export const MCP_TOPIC: SettingsTopic = 'mcp';

/** The computers the desktop app shows, and the code that pairs another one with this one. */
export const COMPUTERS_TOPIC: SettingsTopic = 'computers';

/** How each topic shows in the menu: a new topic does not compile until it is here. */
export const SETTINGS_TOPIC_LABELS: Readonly<Record<SettingsTopic, { label: string; icon: IconName }>> = {
  computers: { icon: 'laptop', label: 'Computers' },
  mcp: { icon: 'plug', label: 'MCP servers' },
  models: { icon: 'cpu', label: 'Models' },
  network: { icon: 'network', label: 'Network' },
  pictures: { icon: 'image', label: 'Pictures' },
  projects: { icon: 'folder', label: 'Projects' },
  ssh: { icon: 'server', label: 'SSH' },
  startup: { icon: 'power', label: 'Startup' },
  thinking: { icon: 'brain', label: 'Thinking' },
  updates: { icon: 'circle-arrow-up', label: 'Updates' },
};

export const isSettingsTopic = (value: string | null | undefined): value is SettingsTopic =>
  SETTINGS_TOPICS.some((topic) => topic === value);
