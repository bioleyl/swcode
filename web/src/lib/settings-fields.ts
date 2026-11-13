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
        { label: 'This computer only', value: DEFAULT },
        { label: 'Every interface: phones and computers on the network too', value: '0.0.0.0' },
      ],
    },
    hint: 'On the network, swcode speaks plain HTTP: whoever sees its link there can use it. A tailnet reaches it on this computer alone, over HTTPS (Settings → Phone).',
    label: 'Listen on',
  },
  SWCODE_IMAGE_URL: {
    control: { kind: 'text', placeholder: 'http://garfield.home:8090' },
    hint: 'A stable-diffusion.cpp sd-server: the agent makes pictures with it, saved in the project, and you see them in the conversation.',
    label: 'Picture server',
  },
  SWCODE_MAX_STEPS: {
    control: { kind: 'text', placeholder: '150' },
    hint: 'How many times the main agent may act in one turn before it stops and reports what it did: 1 to 1000. A local model takes 20 to 40 s a step, so a long task needs many.',
    label: 'Max steps per turn',
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
  SWCODE_SUBAGENT_MAX_STEPS: {
    control: { kind: 'text', placeholder: '30' },
    hint: 'How many times a subagent may act before it stops and reports what it found: 1 to 1000. The main agent waits for its report.',
    label: 'Max steps per subagent',
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
  {
    names: ['SWCODE_THINKING', 'SWCODE_SUBAGENT_THINKING', 'SWCODE_MAX_STEPS', 'SWCODE_SUBAGENT_MAX_STEPS'],
    title: 'Thinking',
    topic: 'thinking',
  },
  { names: ['SWCODE_WORKSPACE'], title: 'Projects', topic: 'projects' },
  { names: ['SWCODE_IMAGE_URL'], title: 'Pictures', topic: 'pictures' },
  { names: ['SWCODE_HOST', 'SWCODE_PORT'], title: 'Network', topic: 'network' },
];
