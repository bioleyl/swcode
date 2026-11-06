/** biome-ignore-all lint/style/useNamingConvention: the settings are environment variable names */
import { THINKING_MODES } from '../../../shared/protocol/choices.schema';
import { THINKING_LABELS } from './thinking-labels';

import type { SettingName } from '../../../shared/protocol/settings.schema';
import type { SettingField, SettingOption, SettingSection } from '../types/settings.types';

const DEFAULT = '';

const thinkingOptions = (defaultMode: keyof typeof THINKING_LABELS): SettingOption[] => [
  { label: `${THINKING_LABELS[defaultMode]} (default)`, value: DEFAULT },
  ...THINKING_MODES.filter((mode) => mode !== defaultMode).map((mode) => ({
    label: THINKING_LABELS[mode],
    value: mode,
  })),
];

/** Every setting the page changes, as it shows it: a new setting does not compile until it is here. */
export const SETTING_FIELDS: Readonly<Record<SettingName, SettingField>> = {
  SWCODE_HOST: {
    control: {
      kind: 'choice',
      options: [
        { label: 'Every interface: phones on the network too', value: DEFAULT },
        { label: 'This computer only', value: '127.0.0.1' },
      ],
    },
    hint: 'Whatever opens swcode needs its access token.',
    label: 'Listen on',
  },
  SWCODE_IMAGE_URL: {
    control: { kind: 'text', placeholder: 'http://garfield.home:8090' },
    hint: 'A stable-diffusion.cpp sd-server: the agent makes pictures with it, saved in the project, and you see them in the conversation.',
    label: 'Picture server',
  },
  SWCODE_PORT: {
    control: { kind: 'text', placeholder: '4500' },
    hint: 'Changed, swcode answers at the new port only: open it there.',
    label: 'Port',
  },
  SWCODE_SSH_ENDPOINTS: {
    control: { kind: 'text', placeholder: 'me@build-box, gmktec' },
    hint: 'Machines projects can be on, reached with ssh, by commas: user@host, or a Host of ~/.ssh/config (those come by themselves). A key or the SSH agent must let ssh in without a password. Linux x64 only, for now.',
    label: 'SSH endpoints',
  },
  SWCODE_SUBAGENT_THINKING: {
    control: { kind: 'choice', options: thinkingOptions('low') },
    hint: 'Their answers are for the main agent: thinking less, they answer sooner.',
    label: 'Subagents',
  },
  SWCODE_THINKING: {
    control: { kind: 'choice', options: thinkingOptions('full') },
    hint: 'Each session can change it in its composer.',
    label: 'New sessions',
  },
  SWCODE_WORKSPACE: {
    control: { kind: 'text', placeholder: 'the folder swcode started in' },
    hint: 'Where projects usually go on this computer: the new project dialog starts there.',
    label: 'Projects folder',
  },
};

export const SETTING_SECTIONS: readonly SettingSection[] = [
  { names: ['SWCODE_THINKING', 'SWCODE_SUBAGENT_THINKING'], title: 'Thinking', topic: 'thinking' },
  { names: ['SWCODE_WORKSPACE'], title: 'Projects', topic: 'projects' },
  { names: ['SWCODE_IMAGE_URL'], title: 'Pictures', topic: 'pictures' },
  { names: ['SWCODE_HOST', 'SWCODE_PORT'], title: 'Network', topic: 'network' },
];
