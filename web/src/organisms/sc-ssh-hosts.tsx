import { createElement, css } from '@jadis/core';
import { confirmDialog, JlAlert, JlButton, JlListItem } from '@syware/jlib';

import { ScElement } from '../base/sc-element';
import { SETTINGS_SAVED } from '../lib/settings-saved';
import { sshHostsLine, sshHostsOf } from '../lib/ssh-hosts';
import { daemonBus } from '../services/daemon';
import { askHosts } from '../services/daemon-answers';
import { refreshStatus } from '../services/daemon-status';
import { loadSettings, saveSettings } from '../services/settings';
import { BASE_STYLES } from '../styles/shared';
import { ScSshHostDialog } from './sc-ssh-host-dialog';
import styles from './sc-ssh-hosts.css?inline';

import type { Tone } from '@syware/jlib';
import type { SettingName } from '../../../shared/protocol/settings.schema';

const SETTING: SettingName = 'SWCODE_SSH_ENDPOINTS';
const UNREADABLE = 'The daemon did not say its settings.';
const NO_HOSTS = 'No SSH host yet: the hosts of ~/.ssh/config come by themselves.';
const FROM_CONFIG = 'A Host of ~/.ssh/config';

/**
 * The settings' SSH: the machines projects can be on, reached with ssh, as a list to add to, change and empty, and
 * under it the hosts of ~/.ssh/config, which come by themselves. Kept in the settings file: each change saves at
 * once, and swcode restarts on it. Set in the environment, the list cannot change here.
 */
export class ScSshHosts extends ScElement {
  static readonly selector = 'sc-ssh-hosts';

  private _hosts: string[] = [];
  // Set from saving until the daemon answers again on the new settings.
  private _restarting = false;

  private readonly _refs = this.useRefs((ref) => ({
    add: ref<JlButton>('.add'),
    configHosts: ref('.config-hosts'),
    configSection: ref('.config'),
    dialog: ref(ScSshHostDialog),
    feedback: ref(JlAlert),
    file: ref('.file'),
    hosts: ref('.hosts'),
  }));

  templateHtml(): Node {
    return (
      <div class="ssh-topic">
        <p class="lead">
          The machines projects can be on, reached with ssh: the new project dialog offers them. A key or the SSH
          agent must let ssh in without a password; Linux x64 only, for now. Kept in{' '}
          <code class="file">~/.swcode/settings.env</code>: each change restarts swcode in a few seconds, once no
          session works.
        </p>
        <JlAlert hidden />
        <section>
          <div class="head">
            <h2>SSH hosts</h2>
            <JlButton class="add" icon="plus" label="Add" size="sm" />
          </div>
          <div class="hosts" />
        </section>
        <section class="config" hidden>
          <h2>From ~/.ssh/config</h2>
          <div class="config-hosts" />
        </section>
        <ScSshHostDialog />
      </div>
    );
  }

  templateCss(): string {
    return css`${BASE_STYLES}${styles}`;
  }

  onConnect(): void {
    this._refs.add.events.register('press', () => this._refs.dialog.edit('', this._hosts));
    this._refs.dialog.events.register('save', ({ host, replaced }) => void this.saveHost(host, replaced));
    this.onBus(daemonBus, 'online', (online) => {
      if (online && this._restarting) {
        void this.restarted();
      }
    });
    void this.load();
  }

  private async load(): Promise<void> {
    const view = await loadSettings();
    if (!this.isConnected) {
      return;
    }
    if (!view) {
      this.say(UNREADABLE, 'danger');
      return;
    }
    this._refs.file.textContent = view.path;
    const fixed = view.environment.includes(SETTING);
    this._refs.add.hidden = fixed;
    this.render(sshHostsOf(view.values[SETTING] ?? ''), fixed);
    if (fixed) {
      this.say(`Set in the environment (${SETTING}): change it there.`, 'info');
    }
    await this.loadFromConfig();
  }

  /** The SSH hosts the daemon offers that the list does not have: those of ~/.ssh/config. */
  private async loadFromConfig(): Promise<void> {
    const hosts = await askHosts();
    if (!hosts.ok || !this.isConnected) {
      return;
    }
    const fromConfig = hosts.answer
      .flatMap((option) => (option.ref.kind === 'ssh' ? [option.ref.endpoint] : []))
      .filter((endpoint) => !this._hosts.includes(endpoint));
    this._refs.configSection.hidden = fromConfig.length === 0;
    this._refs.configHosts.replaceChildren(...fromConfig.map((endpoint) => this.configNode(endpoint)));
  }

  private say(text: string, tone: Tone): void {
    this._refs.feedback.tone.set(tone);
    this._refs.feedback.description.set(text);
    this._refs.feedback.hidden = text === '';
  }

  /** Saves the list as changed: the problem when the daemon refuses it, nothing once it is saved. */
  private async save(hosts: readonly string[]): Promise<string> {
    this.say(SETTINGS_SAVED.saving, 'info');
    const saved = await saveSettings({ [SETTING]: sshHostsLine(hosts) });
    if (!saved.ok) {
      this.say('', 'info');
      return saved.problem;
    }
    this.render([...hosts], false);
    this._restarting = saved.restart === 'self';
    this.say(saved.restart === 'self' ? SETTINGS_SAVED.restarting : SETTINGS_SAVED.byPerson, 'info');
    return '';
  }

  private async saveHost(host: string, replaced: string): Promise<void> {
    const hosts =
      replaced === '' ? [...this._hosts, host] : this._hosts.map((each) => (each === replaced ? host : each));
    const problem = await this.save(hosts);
    if (problem !== '') {
      this._refs.dialog.refuse(problem);
      return;
    }
    this._refs.dialog.close();
  }

  /** A host goes after a confirmation; its projects stay, and open again once it is back. */
  private async removeHost(host: string): Promise<void> {
    const confirmed = await confirmDialog({
      confirmLabel: 'Remove',
      heading: `Remove ${host}?`,
      text: 'New projects can no longer be made there; its projects stay as they are.',
      tone: 'danger',
    });
    if (!confirmed) {
      return;
    }
    const problem = await this.save(this._hosts.filter((each) => each !== host));
    if (problem !== '') {
      this.say(problem, 'danger');
    }
  }

  private async restarted(): Promise<void> {
    this._restarting = false;
    this.say(SETTINGS_SAVED.restarted, 'ok');
    await refreshStatus();
    await this.loadFromConfig();
  }

  private render(hosts: string[], fixed: boolean): void {
    this._hosts = hosts;
    this._refs.hosts.replaceChildren(
      ...(hosts.length === 0 ? [<p class="none">{NO_HOSTS}</p>] : hosts.map((host) => this.hostNode(host, fixed)))
    );
  }

  /** A host, and its buttons unless the environment sets the list: a button keeps its clicks, so each knows its host. */
  private hostNode(host: string, fixed: boolean): Node {
    const row = createElement(JlListItem, { props: { heading: host, icon: 'server' } });
    if (!fixed) {
      row.append(
        this.rowButton('Change', `Change ${host}`, () => this._refs.dialog.edit(host, this._hosts)),
        this.rowButton('Remove', `Remove ${host}`, () => void this.removeHost(host))
      );
    }
    return row;
  }

  private configNode(endpoint: string): Node {
    return createElement(JlListItem, { props: { description: FROM_CONFIG, heading: endpoint, icon: 'server' } });
  }

  private rowButton(label: string, hint: string, press: () => void): JlButton {
    const button = createElement(JlButton, {
      attrs: { slot: 'end' },
      props: { hint, label, size: 'sm', variant: 'ghost' },
    });
    button.events.register('press', press);
    return button;
  }
}

ScSshHosts.register();
