import type { IconName } from './icons';

/** The settings page's topics, in the order its menu lists them. */
export const SETTINGS_TOPICS = ['models', 'projects', 'network', 'startup', 'computers', 'updates'] as const;
export type SettingsTopic = (typeof SETTINGS_TOPICS)[number];

export const DEFAULT_SETTINGS_TOPIC: SettingsTopic = 'models';

/** The computers the desktop app shows, and the code that pairs another one with this one. */
export const COMPUTERS_TOPIC: SettingsTopic = 'computers';

/** How each topic shows in the menu: a new topic does not compile until it is here. */
export const SETTINGS_TOPIC_LABELS: Readonly<Record<SettingsTopic, { label: string; icon: IconName }>> = {
  computers: { icon: 'laptop', label: 'Computers' },
  models: { icon: 'cpu', label: 'Models' },
  network: { icon: 'network', label: 'Network' },
  projects: { icon: 'folder', label: 'Projects' },
  startup: { icon: 'power', label: 'Startup' },
  updates: { icon: 'circle-arrow-up', label: 'Updates' },
};

/** The Models topic's own tabs: the servers, the picture server, the presets and how much the agents think. */
export const MODEL_SETTINGS_TABS = ['llm-servers', 'image-server', 'presets', 'thinking'] as const;
export type ModelSettingsTab = (typeof MODEL_SETTINGS_TABS)[number];

export const MODEL_SETTINGS_TAB_LABELS: Readonly<Record<ModelSettingsTab, { label: string; icon: IconName }>> = {
  'image-server': { icon: 'image', label: 'Image server' },
  'llm-servers': { icon: 'server', label: 'llm-servers' },
  presets: { icon: 'sliders-horizontal', label: 'Presets' },
  thinking: { icon: 'brain', label: 'Thinking' },
};

/** The Network topic's own tabs: the MCP servers, the SSH hosts, and how swcode listens. */
export const NETWORK_SETTINGS_TABS = ['mcp', 'ssh', 'network'] as const;
export type NetworkSettingsTab = (typeof NETWORK_SETTINGS_TABS)[number];

export const NETWORK_SETTINGS_TAB_LABELS: Readonly<Record<NetworkSettingsTab, { label: string; icon: IconName }>> =
  {
    mcp: { icon: 'plug', label: 'MCP servers' },
    network: { icon: 'network', label: 'Network' },
    ssh: { icon: 'server', label: 'SSH' },
  };

/** The MCP servers whose tools the agent has: read while their tab is shown. */
export const MCP_SETTINGS_TAB: NetworkSettingsTab = 'mcp';

/** The Models topic's tabs whose editors read the models while shown. */
export const SOURCES_SETTINGS_TAB: ModelSettingsTab = 'llm-servers';
export const PRESETS_SETTINGS_TAB: ModelSettingsTab = 'presets';

/** Where the settings file's sections show: a topic of its own, or a tab of Models or Network. */
export type SettingsPane = 'projects' | 'image-server' | 'thinking' | 'network';

export const isSettingsTopic = (value: string | null | undefined): value is SettingsTopic =>
  SETTINGS_TOPICS.some((topic) => topic === value);
