import { createElement, css } from '@jadis/core';
import { JlAlert, JlField, JlSelect, JlSwitch } from '@syware/jlib';

import { hostKey } from '../../../shared/hosts/host-key';
import { ClaudeSourcesSchema } from '../../../shared/protocol/claude.schema';
import { ScElement } from '../base/sc-element';
import { loadClaude, saveClaudeSources } from '../services/claude';
import { BASE_STYLES } from '../styles/shared';
import styles from './sc-claude-sources.css?inline';

import type { Tone } from '@syware/jlib';
import type {
  ClaudePluginView,
  ClaudeSkillView,
  ClaudeSources,
  ClaudeView,
} from '../../../shared/protocol/claude.schema';
import type { HostOption } from '../../../shared/protocol/project.schema';

const LOCAL_KEY = 'local';
const LOOKING = 'Looking…';
const SAVED = 'Saved: sessions read them so from their next turn on.';
const NOT_SAVED = 'The daemon did not answer: nothing was saved.';
const UNREACHABLE = 'The daemon did not answer.';
const NO_PLUGINS = 'No plugins installed for Claude Code there.';
const NO_SKILLS = 'No skills or commands in ~/.claude there.';
const FOR_PEOPLE = 'a command for you, in Claude Code: the agent is not offered it';

const isOn = (sources: ClaudeSources, plugin: ClaudePluginView): boolean =>
  sources.plugins[plugin.id] ?? plugin.enabledInClaude;

/** A plugin's line under its name: its id, what Claude Code says of it, and whether this page says otherwise. */
const pluginNote = (sources: ClaudeSources, plugin: ClaudePluginView): string =>
  [
    plugin.id,
    plugin.enabledInClaude ? 'enabled in Claude Code' : 'off in Claude Code',
    ...(sources.plugins[plugin.id] === undefined ? [] : ['set here']),
  ].join(' · ');

/**
 * A plugin turned on or off: back to following Claude Code when that is what Claude Code says, so a plugin enabled
 * there later is read here too.
 */
const withPlugin = (sources: ClaudeSources, plugin: ClaudePluginView, on: boolean): ClaudeSources => {
  const others = Object.fromEntries(Object.entries(sources.plugins).filter(([id]) => id !== plugin.id));
  return { ...sources, plugins: on === plugin.enabledInClaude ? others : { ...others, [plugin.id]: on } };
};

const skillItem = (skill: ClaudeSkillView): Node => (
  <li class="skill">
    <code class="skill-name">{skill.name}</code>
    <span class="skill-description">{skill.forPeople ? FOR_PEOPLE : skill.description}</span>
  </li>
);

/**
 * The settings' Plugins: which of Claude Code's skills and plugins swcode reads besides its own, on the machine a
 * session's project is on. Each change is saved at once; Claude Code's own settings stay as they are.
 */
export class ScClaudeSources extends ScElement {
  static readonly selector = 'sc-claude-sources';

  private _view: ClaudeView | undefined;
  private _sources: ClaudeSources = ClaudeSourcesSchema.parse({});

  private readonly _refs = this.useRefs((ref) => ({
    emptyPlugins: ref('.empty-plugins'),
    emptySkills: ref('.empty-skills'),
    feedback: ref(JlAlert),
    folder: ref('.folder'),
    host: ref(JlSelect),
    hostField: ref(JlField),
    personal: ref<JlSwitch>('.personal'),
    plugins: ref('.plugins'),
    project: ref<JlSwitch>('.project'),
    skills: ref('.skills'),
  }));

  templateHtml(): Node {
    return (
      <div class="claude">
        <p class="lead">
          swcode reads the skills and plugins you installed for Claude Code, on the machine each project is on
          (this computer, WSL, SSH), as Claude Code run there would. Nothing is copied, and Claude Code’s own
          settings stay as they are. swcode’s own skills come first: a Claude Code skill of the same name is left
          out.
        </p>
        <JlAlert hidden />
        <section>
          <h3>What swcode reads</h3>
          <JlSwitch
            class="personal"
            label="Your Claude Code skills"
            description="~/.claude/skills and ~/.claude/commands"
          />
          <JlSwitch
            class="project"
            label="Project skills"
            description="A project’s .claude/skills and .claude/commands, up to its repository’s root"
          />
        </section>
        <JlField label="Claude Code on" hint="Each session reads the machine its project is on." hidden>
          <JlSelect />
        </JlField>
        <p class="found">
          Found in <code class="folder">~/.claude</code>
        </p>
        <section>
          <h3>Plugins</h3>
          <ul class="plugins" aria-label="Plugins" />
          <p class="empty empty-plugins" hidden />
        </section>
        <section>
          <h3>Your Claude Code skills</h3>
          <ul class="skills" aria-label="Your Claude Code skills" />
          <p class="empty empty-skills" hidden />
        </section>
      </div>
    );
  }

  templateCss(): string {
    return css`${BASE_STYLES}${styles}`;
  }

  onConnect(): void {
    this._refs.personal.events.register('change', (on) => void this.save({ ...this._sources, personal: on }));
    this._refs.project.events.register('change', (on) => void this.save({ ...this._sources, project: on }));
    this._refs.host.events.register('change', (key) => void this.load(key));
    if (this._view === undefined) {
      void this.load(LOCAL_KEY);
    }
  }

  private say(text: string, tone: Tone): void {
    this._refs.feedback.tone.set(tone);
    this._refs.feedback.description.set(text);
    this._refs.feedback.hidden = text === '';
  }

  private async load(key: string): Promise<void> {
    this.say(LOOKING, 'info');
    const view = await loadClaude(key);
    if (!this.isConnected) {
      return;
    }
    if (!view) {
      this.say(UNREACHABLE, 'danger');
      return;
    }
    this._view = view;
    this._sources = view.sources;
    this.say(view.problem ?? '', 'warn');
    this._refs.folder.textContent = view.folder;
    this.renderHosts(view.hosts, key);
    this.render();
  }

  /** The machines to look at: shown once there is more than this computer. */
  private renderHosts(hosts: readonly HostOption[], key: string): void {
    this._refs.host.options.set(hosts.map((option) => ({ label: option.label, value: hostKey(option.ref) })));
    this._refs.host.value.set(key);
    this._refs.hostField.hidden = hosts.length < 2;
  }

  private render(): void {
    const view = this._view;
    if (!view) {
      return;
    }
    this._refs.personal.checked.set(this._sources.personal);
    this._refs.project.checked.set(this._sources.project);
    this._refs.plugins.replaceChildren(...view.plugins.map((plugin) => this.pluginItem(plugin)));
    this._refs.emptyPlugins.textContent = NO_PLUGINS;
    this._refs.emptyPlugins.hidden = view.plugins.length > 0;
    this._refs.skills.replaceChildren(...view.personal.map(skillItem));
    this._refs.emptySkills.textContent = NO_SKILLS;
    this._refs.emptySkills.hidden = view.personal.length > 0;
    this._refs.skills.classList.toggle('off', !this._sources.personal);
  }

  /** A plugin and its switch: a switch tells only its own changes, so each knows its plugin. */
  private pluginItem(plugin: ClaudePluginView): Node {
    const on = isOn(this._sources, plugin);
    const name = plugin.version === '' ? plugin.name : `${plugin.name} ${plugin.version}`;
    const toggle = createElement(JlSwitch, {
      props: { checked: on, description: pluginNote(this._sources, plugin), label: name },
    });
    toggle.events.register('change', (checked) => void this.save(withPlugin(this._sources, plugin, checked)));
    return (
      <li class={on ? 'plugin' : 'plugin off'}>
        {toggle}
        {plugin.description !== '' && <span class="plugin-description">{plugin.description}</span>}
        {plugin.skills.length > 0 && <ul class="plugin-skills">{plugin.skills.map(skillItem)}</ul>}
      </li>
    );
  }

  private async save(sources: ClaudeSources): Promise<void> {
    const saved = await saveClaudeSources(sources);
    if (!this.isConnected) {
      return;
    }
    if (!saved) {
      this.say(NOT_SAVED, 'danger');
      this.render();
      return;
    }
    this._sources = saved;
    this.say(this._view?.problem ?? SAVED, this._view?.problem ? 'warn' : 'ok');
    this.render();
  }
}

ScClaudeSources.register();
