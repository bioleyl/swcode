import type { SettingName } from '../../../shared/protocol/settings.schema';
import type { SettingsPane } from '../lib/settings-topics';

/** Which list the models editor shows: the llm-servers, or the presets made from their models. */
export type ModelEditorKind = 'sources' | 'presets';

/** A choice of a select: the value it saves ('' for the default), and what it says. */
export interface SettingOption {
  value: string;
  label: string;
}

/** How the settings page shows a setting: free text (a password for a key), or a choice among fixed options. */
export type SettingControl =
  | { kind: 'text'; placeholder: string; secret?: boolean }
  | { kind: 'choice'; options: readonly SettingOption[] };

export interface SettingField {
  label: string;
  hint: string;
  control: SettingControl;
}

/** A part of the page: a title, and its settings in order, under one of the page's topics or tabs. */
export interface SettingSection {
  title: string;
  names: readonly SettingName[];
  pane: SettingsPane;
}
