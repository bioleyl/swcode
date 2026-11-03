import { createElement, css } from '@jadis/core';
import { JlAlert, JlButton, JlField, JlInput, JlPage, JlSelect, JlTabPanel, JlTabs } from '@syware/jlib';

import { EDITABLE_SETTINGS } from '../../../shared/protocol/settings.schema';
import { ScElement } from '../base/sc-element';
import { SETTING_FIELDS, SETTING_SECTIONS } from '../lib/settings-fields';
import {
  DEFAULT_SETTINGS_TOPIC,
  isSettingsTopic,
  SETTINGS_TOPIC_LABELS,
  SETTINGS_TOPICS,
} from '../lib/settings-topics';
import { ScClaudeSources } from '../organisms/sc-claude-sources';
import { ScComputers } from '../organisms/sc-computers';
import { ScModelsEditor } from '../organisms/sc-models-editor';
import { ScSkillsEditor } from '../organisms/sc-skills-editor';
import { ScStartupSettings } from '../organisms/sc-startup-settings';
import { daemonBus } from '../services/daemon';
import { refreshStatus } from '../services/daemon-status';
import { inDesktopApp } from '../services/desktop';
import { hrefOf, settingsPath } from '../services/navigation';
import { loadSettings, saveSettings } from '../services/settings';
import { loadStartup } from '../services/startup';
import { BASE_STYLES } from '../styles/shared';
import styles from './settings-page.css?inline';

import type { SelectOption, Tone } from '@syware/jlib';
import type { SettingName, SettingsView, SettingValues } from '../../../shared/protocol/settings.schema';
import type { SettingsTopic } from '../lib/settings-topics';
import type { SettingControl, SettingOption, SettingSection } from '../types/settings.types';

type Control = JlInput | JlSelect;

const APP_NAME = 'swcode';
const PORT_SETTING: SettingName = 'SWCODE_PORT';
const DEFAULT_PORT = '4500';
// A setting left empty takes its default; jlib's select takes an empty value for nothing chosen, so that choice has
// a value of its own on the page.
const DEFAULT_CHOICE = 'default';
const SAVING = 'Saving…';
const RESTARTING = 'Saved. swcode restarts on them: a few seconds…';
const RESTARTED = 'Saved: swcode runs with them now.';
const BY_PERSON = 'Saved. Restart swcode serve for them to apply.';
const SECRET_TYPE = 'password';
// The topics the settings file holds: the others (skills, plugins…) save as they change.
const FILE_TOPICS: ReadonlySet<SettingsTopic> = new Set(SETTING_SECTIONS.map((section) => section.topic));

const toChoice = (value: string): string => (value === '' ? DEFAULT_CHOICE : value);
const fromChoice = (value: string): string => (value === DEFAULT_CHOICE ? '' : value);
const choiceOf = (option: SettingOption): SelectOption => ({ label: option.label, value: toChoice(option.value) });

// How each kind of control is made.
const CONTROLS: {
  [Kind in SettingControl['kind']]: (control: Extract<SettingControl, { kind: Kind }>) => Control;
} = {
  choice: (control) => createElement(JlSelect, { props: { options: control.options.map(choiceOf) } }),
  text: (control) =>
    createElement(JlInput, {
      props: {
        autocomplete: 'off',
        placeholder: control.placeholder,
        type: control.secret ? SECRET_TYPE : 'text',
      },
    }),
};

const controlFrom = <Kind extends SettingControl['kind']>(
  kind: Kind,
  control: Extract<SettingControl, { kind: Kind }>
): Control => CONTROLS[kind](control);

const controlOf = (control: SettingControl): Control => controlFrom(control.kind, control);

const controlValue = (control: Control): string => fromChoice(control.value.get().trim());

/** The panels of the topics kept elsewhere than in the settings file, each its own organism. */
const ORGANISM_PANELS: Readonly<Partial<Record<SettingsTopic, () => Node>>> = {
  computers: () => <ScComputers />,
  models: () => <ScModelsEditor />,
  plugins: () => <ScClaudeSources />,
  skills: () => <ScSkillsEditor />,
  startup: () => <ScStartupSettings />,
};

const portNote = (port: string): string =>
  `Saved. swcode now answers on port ${port}: open http://${location.hostname}:${port}/ (the access token stays).`;

/**
 * The settings of this swcode, a tab for each topic, each with its own address: how much the agents think, where
 * projects go, pictures, how it listens, kept in its settings file and saved together; the llama-servers and the
 * presets, how it starts, the other computers, the skills, and Claude Code's skills and plugins, each saved as it
 * changes. Saving the file restarts the daemon on it; a setting set in the environment cannot change.
 */
export class SettingsPage extends ScElement {
  static readonly selector = 'settings-page';

  private readonly _controls = new Map<SettingName, Control>();
  private readonly _fields = new Map<SettingName, JlField>();
  private readonly _panels = new Map<SettingsTopic, JlTabPanel>();
  private _view: SettingsView | undefined;
  // Set from saving until the daemon answers again on the new settings.
  private _restarting = false;

  readonly route = this.useAttributes({
    topic: (topic) => this.showTopic(isSettingsTopic(topic) ? topic : DEFAULT_SETTINGS_TOPIC),
  });

  private readonly _refs = this.useRefs((ref) => ({
    feedback: ref(JlAlert),
    save: ref<JlButton>('.save'),
    tabs: ref(JlTabs),
  }));

  templateHtml(): Node {
    return (
      <JlPage
        heading="Settings"
        description="How this swcode works, by topic. A setting set in the environment cannot change here."
        appName={APP_NAME}>
        <JlButton slot="actions" class="save" variant="primary" label="Save" />
        <JlAlert hidden />
        <JlTabs label="Topics">{SETTINGS_TOPICS.map((topic) => this.panelNode(topic))}</JlTabs>
      </JlPage>
    );
  }

  templateCss(): string {
    return css`${BASE_STYLES}${styles}`;
  }

  onConnect(): void {
    this._refs.tabs.events.register('change', (topic) => this.pickTopic(topic));
    this._refs.save.events.register('press', () => void this.save());
    this.onBus(daemonBus, 'online', (online) => {
      if (online && this._restarting) {
        void this.restarted();
      }
    });
    if (!inDesktopApp()) {
      this.panelFor('computers').remove();
    }
    this.renderSave(this.topicShown());
    void this.load();
    void this.offerStartup();
  }

  private panelNode(topic: SettingsTopic): Node {
    const look = SETTINGS_TOPIC_LABELS[topic];
    const content = ORGANISM_PANELS[topic]?.() ?? this.fileTopicNode(topic);
    const panel = createElement(JlTabPanel, { props: { icon: look.icon, label: look.label, name: topic } });
    panel.append(content);
    this._panels.set(topic, panel);
    return panel;
  }

  private fileTopicNode(topic: SettingsTopic): Node {
    return (
      <div class="file-topic">
        <p class="lead">
          Kept in <code class="file">~/.swcode/settings.env</code>. Saving restarts swcode on them in a few
          seconds; it waits while a session works.
        </p>
        {SETTING_SECTIONS.filter((section) => section.topic === topic).map((section) => this.sectionNode(section))}
      </div>
    );
  }

  private panelFor(topic: SettingsTopic): JlTabPanel {
    const panel = this._panels.get(topic);
    if (!panel) {
      throw new Error(`No panel for ${topic}`);
    }
    return panel;
  }

  /** The Startup topic shows where swcode can start with its computer: in the Windows app. */
  private async offerStartup(): Promise<void> {
    const startup = await loadStartup();
    if (!this.isConnected || startup?.available) {
      return;
    }
    this.panelFor('startup').remove();
  }

  private topicShown(): SettingsTopic {
    const value = this._refs.tabs.value.get();
    return isSettingsTopic(value) ? value : DEFAULT_SETTINGS_TOPIC;
  }

  /** A tab picked: the address follows, without leaving the page and what its form holds. */
  private pickTopic(topic: string): void {
    if (!isSettingsTopic(topic)) {
      return;
    }
    history.replaceState(history.state, '', hrefOf(settingsPath(topic)));
    this.renderSave(topic);
  }

  private showTopic(topic: SettingsTopic): void {
    this._refs.tabs.value.set(topic);
    this.renderSave(topic);
  }

  /** Save is for the settings file: the other topics save as they change. */
  private renderSave(topic: SettingsTopic): void {
    this._refs.save.hidden = !FILE_TOPICS.has(topic);
  }

  private sectionNode(section: SettingSection): Node {
    return (
      <section>
        <h2>{section.title}</h2>
        {section.names.map((name) => this.fieldNode(name))}
      </section>
    );
  }

  private fieldNode(name: SettingName): Node {
    const field = SETTING_FIELDS[name];
    const control = controlOf(field.control);
    control.id = `setting-${name}`;
    this._controls.set(name, control);
    const node = createElement(JlField, { props: { hint: field.hint, label: field.label } });
    node.append(control);
    this._fields.set(name, node);
    return node;
  }

  private controlFor(name: SettingName): Control {
    const control = this._controls.get(name);
    if (!control) {
      throw new Error(`No control for ${name}`);
    }
    return control;
  }

  private async load(): Promise<void> {
    const view = await loadSettings();
    if (!view || !this.isConnected) {
      return;
    }
    this._view = view;
    for (const file of this.shadowRoot?.querySelectorAll('.file') ?? []) {
      file.textContent = view.path;
    }
    for (const name of EDITABLE_SETTINGS) {
      this.show(name, view);
    }
  }

  /** A setting's value; set in the environment, it cannot change here, and says so. */
  private show(name: SettingName, view: SettingsView): void {
    const control = this.controlFor(name);
    const value = view.values[name] ?? '';
    this.offer(control, value);
    control.value.set(control instanceof JlSelect ? toChoice(value) : value);
    const fixed = view.environment.includes(name);
    control.disabled.set(fixed);
    if (fixed) {
      this._fields.get(name)?.hint.set(`Set in the environment (${name}): change it there.`);
    }
  }

  /** A select keeps a value it does not list: a model llama-server does not serve right now, say. */
  private offer(control: Control, value: string): void {
    if (!(control instanceof JlSelect) || value === '') {
      return;
    }
    const options = control.options.get();
    if (options.some((option) => option.value === value)) {
      return;
    }
    control.options.set([...options, { label: value, value }]);
  }

  /** What the page holds: an emptied field goes back to its default; one set in the environment stays out. */
  private values(): SettingValues {
    return Object.fromEntries(
      EDITABLE_SETTINGS.flatMap((name) => {
        const control = this.controlFor(name);
        return control.disabled.get() ? [] : [[name, controlValue(control)]];
      })
    );
  }

  private say(text: string, tone: Tone): void {
    this._refs.feedback.tone.set(tone);
    this._refs.feedback.description.set(text);
    this._refs.feedback.hidden = text === '';
  }

  private async save(): Promise<void> {
    const values = this.values();
    this.say(SAVING, 'info');
    this._refs.save.loading.set(true);
    const saved = await saveSettings(values);
    this._refs.save.loading.set(false);
    if (!saved.ok) {
      this.say(saved.problem, 'danger');
      return;
    }
    const port = values[PORT_SETTING] || DEFAULT_PORT;
    const before = this._view?.values[PORT_SETTING] || DEFAULT_PORT;
    if (port !== before) {
      this.say(portNote(port), 'ok');
      return;
    }
    this._restarting = saved.restart === 'self';
    this.say(saved.restart === 'self' ? RESTARTING : BY_PERSON, 'info');
  }

  private async restarted(): Promise<void> {
    this._restarting = false;
    this.say(RESTARTED, 'ok');
    await refreshStatus();
    await this.load();
  }
}

SettingsPage.register();
