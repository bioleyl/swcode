import { css } from '@jadis/core';
import { JlAlert, JlButton, JlDialog, JlField, JlInput, JlSegmented, JlSelect } from '@syware/jlib';

import { hostKey, hostLabel } from '../../../shared/hosts/host-key';
import { LOCAL_HOST, NewFolderNameSchema } from '../../../shared/protocol/project.schema';
import { ScElement } from '../base/sc-element';
import { childPath } from '../lib/folders';
import { folderName } from '../lib/format';
import { ScFolderBrowser } from '../molecules/sc-folder-browser';
import { askCreateProject, askFolders, askHosts } from '../services/daemon-answers';
import { BASE_STYLES } from '../styles/shared';
import styles from './sc-project-dialog.css?inline';

import type { SegmentedOption } from '@syware/jlib';
import type { HostOption, HostRef, Project } from '../../../shared/protocol/project.schema';
import type { ProjectSource, SourceView } from '../types/project.types';

const SOURCES: Readonly<Record<ProjectSource, SourceView>> = {
  existing: {
    confirm: (folder) => `Use ${folderName(folder)}`,
    lead: 'Go to the folder the project works in, then use it.',
  },
  fresh: {
    confirm: (_folder, name) => (name === '' ? 'Create the folder' : `Create ${name}`),
    lead: 'Name the new folder, and go to the one it goes in.',
  },
};

const LOCAL_OPTION: HostOption = { label: hostLabel(LOCAL_HOST), ref: LOCAL_HOST };

const SOURCE_OPTIONS: SegmentedOption<ProjectSource>[] = [
  { label: 'Existing folder', value: 'existing' },
  { label: 'New folder', value: 'fresh' },
];

const sourceOf = (value: string): ProjectSource => (value === 'fresh' ? 'fresh' : 'existing');

/**
 * Makes a project: of a folder that exists, or of a new one made there, on the daemon's computer, one of its WSL
 * distributions or an SSH endpoint. `created` gives the project once the daemon made it.
 */
export class ScProjectDialog extends ScElement {
  static readonly selector = 'sc-project-dialog';

  readonly events = this.useEvents<{ created: Project }>();

  private _source: ProjectSource = 'existing';
  // Where the project goes, among the hosts the daemon offers.
  private _host: HostRef = LOCAL_HOST;
  private _hosts: HostOption[] = [LOCAL_OPTION];
  // Set while the daemon makes the project: its answer is for this dialog.
  private _making = false;

  private readonly _refs = this.useRefs((ref) => ({
    browser: ref(ScFolderBrowser),
    cancel: ref<JlButton>('.cancel'),
    confirm: ref<JlButton>('.confirm'),
    dialog: ref(JlDialog),
    host: ref(JlSelect),
    hostField: ref<JlField>('.where'),
    lead: ref('.lead'),
    name: ref(JlInput),
    nameField: ref<JlField>('.name'),
    problem: ref(JlAlert),
    sources: ref(JlSegmented),
    target: ref('.target'),
  }));

  templateHtml(): Node {
    return (
      <JlDialog heading="New project" size="lg">
        <div class="body">
          <JlField class="where" label="Where" hidden>
            <JlSelect />
          </JlField>
          <JlSegmented label="Folder" options={SOURCE_OPTIONS} value="existing" />
          <p class="lead" />
          <JlField class="name" label="Folder name">
            <JlInput autocomplete="off" placeholder="my-project" />
          </JlField>
          <ScFolderBrowser />
          <p class="target" />
          <JlAlert tone="danger" hidden />
        </div>
        <JlButton slot="actions" class="cancel" label="Cancel" />
        <JlButton slot="actions" class="confirm" variant="primary" />
      </JlDialog>
    );
  }

  templateCss(): string {
    return css`${BASE_STYLES}${styles}`;
  }

  onConnect(): void {
    this._refs.cancel.events.register('press', () => this._refs.dialog.close());
    this._refs.confirm.events.register('press', () => void this.make());
    this._refs.browser.events.register('go', (path) => void this.goTo(path));
    this._refs.sources.events.register('change', (value) => this.choose(sourceOf(value)));
    this._refs.name.events.register('input', () => this.render());
    this._refs.host.events.register('change', (key) => void this.chooseHost(key));
  }

  open(): void {
    this._refs.name.value.set('');
    this.showProblem('');
    this._host = LOCAL_HOST;
    this.renderHosts();
    void this.loadHosts();
    this.choose('existing');
    void this.goTo(undefined);
    this._refs.dialog.show();
  }

  /** The hosts to choose from, from the daemon: it lists only this computer when it knows no other. */
  private async loadHosts(): Promise<void> {
    const hosts = await askHosts();
    if (!hosts.ok || !this.isConnected) {
      return;
    }
    this._hosts = hosts.answer;
    this.renderHosts();
  }

  private showProblem(text: string): void {
    this._refs.problem.description.set(text);
    this._refs.problem.hidden = text === '';
  }

  /** Another host: its folders, from its usual folder, take the browser's place. */
  private async chooseHost(key: string): Promise<void> {
    this._host = this._hosts.find((option) => hostKey(option.ref) === key)?.ref ?? LOCAL_HOST;
    this.showProblem('');
    await this.goTo(undefined);
  }

  /** The hosts to choose from: shown once there is more than this computer. */
  private renderHosts(): void {
    this._refs.host.options.set(
      this._hosts.map((option) => ({ label: option.label, value: hostKey(option.ref) }))
    );
    this._refs.host.value.set(hostKey(this._host));
    this._refs.hostField.hidden = this._hosts.length < 2;
  }

  private choose(source: ProjectSource): void {
    this._source = source;
    this._refs.sources.value.set(source);
    this._refs.nameField.hidden = source !== 'fresh';
    this._refs.lead.textContent = SOURCES[source].lead;
    this.render();
    if (source === 'fresh') {
      this._refs.name.focus();
    }
  }

  /** Asks for a folder's subfolders; without a path, for the folder projects usually go in. */
  private async goTo(path: string | undefined): Promise<void> {
    this._refs.browser.loading();
    this.render();
    const folders = await askFolders(this._host, path);
    if (!this.isConnected || !this._refs.dialog.open.get()) {
      return;
    }
    if (!folders.ok) {
      this._refs.browser.failed(folders.problem);
      this.render();
      return;
    }
    this._refs.browser.show(folders.answer);
    this.render();
  }

  private async make(): Promise<void> {
    const folder = this._refs.browser.path;
    if (folder === undefined) {
      return;
    }
    this._making = true;
    this.showProblem('');
    this.render();
    const newFolder = this._source === 'fresh' ? this.newName() : undefined;
    const created = await askCreateProject(folder, this._host, newFolder);
    this._making = false;
    if (!this.isConnected) {
      return;
    }
    if (!created.ok) {
      this.showProblem(created.problem);
      this.render();
      return;
    }
    this._refs.dialog.close();
    this.events.emit('created', created.answer);
  }

  /** The new folder's name, once it is one: a name, not a path. */
  private newName(): string | undefined {
    const parsed = NewFolderNameSchema.safeParse(this._refs.name.value.get());
    return parsed.success ? parsed.data : undefined;
  }

  private render(): void {
    const folder = this._refs.browser.path;
    const name = this._refs.name.value.get().trim();
    const fresh = this._source === 'fresh';
    const ready = folder !== undefined && (!fresh || this.newName() !== undefined);
    this._refs.confirm.label.set(SOURCES[this._source].confirm(folder ?? '', name));
    this._refs.confirm.disabled.set(!ready || this._making);
    this._refs.target.textContent =
      fresh && folder !== undefined && name !== '' ? `Makes ${childPath(folder, name)}` : '';
  }
}

ScProjectDialog.register();
