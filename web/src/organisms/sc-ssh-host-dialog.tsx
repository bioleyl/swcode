import { css } from '@jadis/core';
import { JlAlert, JlButton, JlDialog, JlField, JlInput } from '@syware/jlib';

import { ScElement } from '../base/sc-element';
import { sshHostProblem } from '../lib/ssh-hosts';
import { BASE_STYLES } from '../styles/shared';
import styles from './sc-ssh-host-dialog.css?inline';

import type { SshHost } from '../lib/ssh-hosts';

/**
 * Adds an SSH host or changes one: a name people read it by, and user@host, or a Host of ~/.ssh/config. `save` gives
 * the host, and the one it replaces (nothing for a new one); the list saves it, then closes the dialog or shows why
 * not.
 */
export class ScSshHostDialog extends ScElement {
  static readonly selector = 'sc-ssh-host-dialog';

  readonly events = this.useEvents<{ save: { host: SshHost; replaced?: SshHost } }>();

  // The host changed, nothing for a new one; the others in the list.
  private _replaced: SshHost | undefined;
  private _taken: readonly SshHost[] = [];

  private readonly _refs = this.useRefs((ref) => ({
    cancel: ref<JlButton>('.cancel'),
    dialog: ref(JlDialog),
    host: ref<JlInput>('.host'),
    name: ref<JlInput>('.name'),
    problem: ref(JlAlert),
    save: ref<JlButton>('.save'),
  }));

  templateHtml(): Node {
    return (
      <JlDialog heading="SSH host">
        <div class="body">
          <JlField label="Name" hint="What the app calls it: in the new project dialog, and in its list here.">
            <JlInput class="name" autocomplete="off" placeholder="buildbox" maxlength="40" />
          </JlField>
          <JlField label="Host" hint="user@host, or a Host of ~/.ssh/config: ssh must get in without a password.">
            <JlInput class="host" autocomplete="off" placeholder="me@build-box" />
          </JlField>
          <JlAlert tone="danger" hidden />
        </div>
        <JlButton slot="actions" class="cancel" label="Cancel" />
        <JlButton slot="actions" class="save" variant="primary" label="Save" />
      </JlDialog>
    );
  }

  templateCss(): string {
    return css`${BASE_STYLES}${styles}`;
  }

  onConnect(): void {
    this._refs.cancel.events.register('press', () => this.close());
    this._refs.save.events.register('press', () => this.submit());
  }

  /** Opens on a host to change, or empty for a new one. */
  edit(host: SshHost | undefined, hosts: readonly SshHost[]): void {
    this._replaced = host;
    this._taken = hosts.filter((each) => each !== host);
    const name = host?.name ?? '';
    this._refs.dialog.heading.set(name === '' ? 'Add an SSH host' : `Change ${name}`);
    this._refs.name.value.set(name);
    this._refs.host.value.set(host?.endpoint ?? '');
    this.refuse('');
    this._refs.dialog.show();
  }

  /** Why the list could not save it: the dialog stays open on it. */
  refuse(problem: string): void {
    this._refs.save.loading.set(false);
    this._refs.problem.description.set(problem);
    this._refs.problem.hidden = problem === '';
  }

  close(): void {
    this._refs.dialog.close();
  }

  private submit(): void {
    const endpoint = this._refs.host.value.get().trim();
    const typed = this._refs.name.value.get().trim();
    // A host left unnamed goes by its endpoint.
    const host: SshHost = { endpoint, name: typed === '' ? endpoint : typed };
    const problem = sshHostProblem(host, this._taken);
    if (problem !== '') {
      this.refuse(problem);
      return;
    }
    this._refs.save.loading.set(true);
    this.events.emit('save', { host, ...(this._replaced === undefined ? {} : { replaced: this._replaced }) });
  }
}

ScSshHostDialog.register();
