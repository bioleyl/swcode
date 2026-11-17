import { createElement, css } from '@jadis/core';
import { JlAlert, JlButton, JlField, JlInput, JlPage, JlSelect, JlTabPanel, JlTabs } from '@syware/jlib';

import { ScElement } from '../base/sc-element';
import { SETTING_FIELDS, SETTING_SECTIONS } from '../lib/settings-fields';
import { SETTINGS_SAVED } from '../lib/settings-saved';
import {
  DEFAULT_SETTINGS_TOPIC,
  isSettingsTopic,
  MCP_SETTINGS_TAB,
  MODEL_SETTINGS_TAB_LABELS,
  MODEL_SETTINGS_TABS,
  NETWORK_SETTINGS_TAB_LABELS,
  NETWORK_SETTINGS_TABS,
  PRESETS_SETTINGS_TAB,
  SETTINGS_TOPIC_LABELS,
  SETTINGS_TOPICS,
  SOURCES_SETTINGS_TAB,
} from '../lib/settings-topics';
import { ScAppUpdateSettings } from '../organisms/sc-app-update-settings';
import { ScComputers } from '../organisms/sc-computers';
import { ScMcpServers } from '../organisms/sc-mcp-servers';
import { ScModelsEditor } from '../organisms/sc-models-editor';
import { ScSshHosts } from '../organisms/sc-ssh-hosts';
import { ScStartupSettings } from '../organisms/sc-startup-settings';
import { ScUpdateSettings } from '../organisms/sc-update-settings';
import { daemonBus } from '../services/daemon';
import { refreshStatus } from '../services/daemon-status';
import { inAndroidApp } from '../services/native-app';
import { hrefOf, settingsPath } from '../services/navigation';
import { confirmRestart, loadSettings, saveSettings } from '../services/settings';
import { loadStartup } from '../services/startup';
import { offersUpdates } from '../services/updates';
import { BASE_STYLES } from '../styles/shared';
import styles from './settings-page.css?inline';

import type { SelectOption, Tone } from '@syware/jlib';
import type { SettingName, SettingsView, SettingValues } from '../../../shared/protocol/settings.schema';
import type { IconName } from '../lib/icons';
import type { ModelSettingsTab, NetworkSettingsTab, SettingsPane, SettingsTopic } from '../lib/settings-topics';
import type { SettingControl, SettingOption, SettingSection } from '../types/settings.types';

type Control = JlInput | JlSelect;

const APP_NAME = 'swcode';
const MODELS_TOPIC: SettingsTopic = 'models';
const NETWORK_TOPIC: SettingsTopic = 'network';
const PROJECTS_TOPIC: SettingsTopic = 'projects';
const PROJECTS_PANE: SettingsPane = 'projects';
const PORT_SETTING: SettingName = 'SWCODE_PORT';
const DEFAULT_PORT = '4500';
// A setting left empty takes its default; jlib's select takes an empty value for nothing chosen, so that choice has
// a value of its own on the page.
const DEFAULT_CHOICE = 'default';
const SECRET_TYPE = 'password';
// The settings of the topics and tabs the page's form holds, saved together: the others save as they change.
const FORM_SETTINGS: readonly SettingName[] = SETTING_SECTIONS.flatMap((section) => section.names);
const FILE_PANES: ReadonlySet<string> = new Set(SETTING_SECTIONS.map((section) => section.pane));

const toChoice = (value: string): string => (value === '' ? DEFAULT_CHOICE : value);
const fromChoice = (value: string): string => (value === DEFAULT_CHOICE ? '' : value);
const choiceOf = (option: SettingOption): SelectOption => ({ label: option.label, value: option.value });

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

const portNote = (port: string): string =>
  `Saved. swcode now answers on port ${port}: open http://${location.hostname}:${port}/ (the access token stays).`;

/**
 * The settings of this swcode, a tab for each topic, each with its own address: the models, their llm-servers, the
 * picture server, the presets and how much the agents think; where projects go; the MCP servers, the SSH hosts and how
 * it listens; how it starts, the other computers and its updates. The settings file's topics save together; the others
 * save as they change. Saving the file restarts the daemon on it; a setting set in the environment cannot change.
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
    mcp: ref(ScMcpServers),
    modelTabs: ref<JlTabs>('.model-tabs'),
    networkTabs: ref<JlTabs>('.network-tabs'),
    presetsEditor: ref<ScModelsEditor>('.presets-editor'),
    save: ref<JlButton>('.save'),
    sourcesEditor: ref<ScModelsEditor>('.sources-editor'),
    tabs: ref(JlTabs),
  }));

  /** The tab whose settings file the Save button saves: the topic itself, or the tab of the topic shown. */
  private readonly _paneOf: Readonly<Partial<Record<SettingsTopic, () => string>>> = {
    [MODELS_TOPIC]: () => this._refs.modelTabs.value.get(),
    [NETWORK_TOPIC]: () => this._refs.networkTabs.value.get(),
    [PROJECTS_TOPIC]: () => PROJECTS_PANE,
  };

  /** What each topic shows: its settings, or the tabs of the models and the network. */
  private readonly _topicNodes: Readonly<Record<SettingsTopic, () => Node>> = {
    computers: () => <ScComputers />,
    models: () => this.modelsNode(),
    network: () => this.networkNode(),
    projects: () => this.filePaneNode(PROJECTS_PANE),
    startup: () => <ScStartupSettings />,
    updates: () => (inAndroidApp() ? <ScAppUpdateSettings /> : <ScUpdateSettings />),
  };

  /** What each tab of the Models topic shows. */
  private readonly _modelTabNodes: Readonly<Record<ModelSettingsTab, () => Node>> = {
    'image-server': () => this.filePaneNode('image-server'),
    'llm-servers': () => <ScModelsEditor class="sources-editor" kind="sources" />,
    presets: () => <ScModelsEditor class="presets-editor" kind="presets" />,
    thinking: () => this.filePaneNode('thinking'),
  };

  /** What each tab of the Network topic shows. */
  private readonly _networkTabNodes: Readonly<Record<NetworkSettingsTab, () => Node>> = {
    mcp: () => <ScMcpServers />,
    network: () => this.filePaneNode('network'),
    ssh: () => <ScSshHosts />,
  };

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
    this._refs.modelTabs.events.register('change', () => this.renderTopic(MODELS_TOPIC));
    this._refs.networkTabs.events.register('change', () => this.renderTopic(NETWORK_TOPIC));
    this._refs.save.events.register('press', () => void this.save());
    this.onBus(daemonBus, 'online', (online) => {
      if (online && this._restarting) {
        void this.restarted();
      }
    });
    if (!offersUpdates()) {
      this.panelFor('updates').remove();
    }
    this.renderTopic(this.topicShown());
    void this.load();
    void this.offerStartup();
  }

  private panelNode(topic: SettingsTopic): Node {
    const panel = createElement(JlTabPanel, {
      props: { icon: SETTINGS_TOPIC_LABELS[topic].icon, label: SETTINGS_TOPIC_LABELS[topic].label, name: topic },
    });
    panel.append(this._topicNodes[topic]());
    this._panels.set(topic, panel);
    return panel;
  }

  /** The Models topic: its llm-servers, its picture server, its presets and how much the agents think. */
  private modelsNode(): Node {
    return (
      <JlTabs class="model-tabs" label="Models" value={SOURCES_SETTINGS_TAB}>
        {MODEL_SETTINGS_TABS.map((tab) =>
          this.tabNode(tab, MODEL_SETTINGS_TAB_LABELS[tab], this._modelTabNodes[tab]())
        )}
      </JlTabs>
    );
  }

  /** The Network topic: the MCP servers, the SSH hosts, and how swcode listens. */
  private networkNode(): Node {
    return (
      <JlTabs class="network-tabs" label="Network" value={MCP_SETTINGS_TAB}>
        {NETWORK_SETTINGS_TABS.map((tab) =>
          this.tabNode(tab, NETWORK_SETTINGS_TAB_LABELS[tab], this._networkTabNodes[tab]())
        )}
      </JlTabs>
    );
  }

  private tabNode(
    name: ModelSettingsTab | NetworkSettingsTab,
    look: { label: string; icon: IconName },
    content: Node
  ): JlTabPanel {
    const panel = createElement(JlTabPanel, { props: { icon: look.icon, label: look.label, name } });
    panel.append(content);
    return panel;
  }

  private filePaneNode(pane: SettingsPane): Node {
    return (
      <div class="file-pane">
        <p class="lead">
          Kept in <code class="file">~/.swcode/settings.env</code>. Saving restarts swcode on them in a few
          seconds; it waits while a session works.
        </p>
        {SETTING_SECTIONS.filter((section) => section.pane === pane).map((section) => this.sectionNode(section))}
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
    this.renderTopic(topic);
  }

  private showTopic(topic: SettingsTopic): void {
    this._refs.tabs.value.set(topic);
    this.renderTopic(topic);
  }

  /** Save is for the settings file: the other topics and tabs save as they change. */
  private renderTopic(topic: SettingsTopic): void {
    const pane = this._paneOf[topic]?.();
    this._refs.save.hidden = pane === undefined || !FILE_PANES.has(pane);
    this._refs.mcp.active.set(topic === NETWORK_TOPIC && this._refs.networkTabs.value.get() === MCP_SETTINGS_TAB);
    // The models' editors read the models while their tab is shown: the other one may have changed them.
    const modelTab = this._refs.modelTabs.value.get();
    this._refs.sourcesEditor.active.set(topic === MODELS_TOPIC && modelTab === SOURCES_SETTINGS_TAB);
    this._refs.presetsEditor.active.set(topic === MODELS_TOPIC && modelTab === PRESETS_SETTINGS_TAB);
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
    for (const name of FORM_SETTINGS) {
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

  /** A select keeps a value it does not list: a model llm-server does not serve right now, say. */
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
      FORM_SETTINGS.flatMap((name) => {
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
    if (!(await confirmRestart(this._view?.restart))) {
      return;
    }
    const values = this.values();
    this.say(SETTINGS_SAVED.saving, 'info');
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
    this.say(saved.restart === 'self' ? SETTINGS_SAVED.restarting : SETTINGS_SAVED.byPerson, 'info');
  }

  private async restarted(): Promise<void> {
    this._restarting = false;
    this.say(SETTINGS_SAVED.restarted, 'ok');
    await refreshStatus();
    await this.load();
  }
}

SettingsPage.register();
