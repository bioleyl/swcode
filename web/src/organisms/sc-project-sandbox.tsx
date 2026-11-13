import { css } from '@jadis/core';
import { JlAlert, JlCard, JlSwitch } from '@syware/jlib';

import { ScElement } from '../base/sc-element';
import { setProjectTrust } from '../services/project-list';
import { readSandbox, setProjectSandbox } from '../services/sandbox';
import { BASE_STYLES } from '../styles/shared';
import styles from './sc-project-sandbox.css?inline';

import type { Project, SandboxView } from '../../../shared/protocol/project.schema';

const NOTE =
  'What this folder may do in your name. Its AGENTS.md and its own skills are instructions the agent follows as yours: they are given to it only once you trust the folder. In a sandbox, the agent’s commands write only in the project’s folder, its scratch folder and a /tmp of their own, and never see your keys (~/.ssh, cloud and git credentials): a command that needs more asks first, even in a session that runs everything.';
const UNREACHABLE = 'The daemon did not answer.';
const TRUST_NOTE = 'Give its AGENTS.md and its skills to the agent.';
const SANDBOX_NOTES = {
  free: 'Turn it off for commands that must write elsewhere without asking.',
  kept: 'A folder that is not trusted keeps its sandbox.',
} as const;

/**
 * A project's trust and sandbox, in its Config tab: whether the person trusts its folder (what it says of itself is
 * given to the agent only then), whether its sessions' commands run in a sandbox, which they do unless turned off
 * here, and why they cannot where the project's host makes none. It asks the daemon about the host when the project
 * changes.
 */
export class ScProjectSandbox extends ScElement {
  static readonly selector = 'sc-project-sandbox';

  readonly project = this.useChange<Project | undefined>(undefined, () => this.render());

  // The project the host was asked about, and what it said.
  private _askedFor: string | undefined;
  private _view: SandboxView | undefined;

  private readonly _refs = this.useRefs((ref) => ({
    problem: ref(JlAlert),
    switch: ref<JlSwitch>('.sandboxed'),
    trust: ref<JlSwitch>('.trusted'),
  }));

  templateHtml(): Node {
    return (
      <JlCard class="sandbox" heading="Trust and sandbox" description={NOTE}>
        <div class="body">
          <JlSwitch class="trusted" label="Trust this folder" description={TRUST_NOTE} />
          <JlSwitch class="sandboxed" label="Run commands in a sandbox" />
          <JlAlert tone="warn" hidden />
        </div>
      </JlCard>
    );
  }

  templateCss(): string {
    return css`${BASE_STYLES}${styles}`;
  }

  onConnect(): void {
    this._refs.switch.events.register('change', (on) => {
      const project = this.project.get();
      if (project) {
        setProjectSandbox(project.id, on);
      }
    });
    this._refs.trust.events.register('change', (trusted) => {
      const project = this.project.get();
      if (project) {
        setProjectTrust(project.id, trusted);
      }
    });
    this.render();
  }

  private render(): void {
    const project = this.project.get();
    if (!project || !this.isConnected) {
      return;
    }
    if (this._askedFor !== project.id) {
      this._askedFor = project.id;
      this._view = undefined;
      void this.ask(project.id);
    }
    const view = this._view;
    const available = view?.available ?? false;
    const untrusted = project.untrusted === true;
    this._refs.trust.checked.set(!untrusted);
    // Where the host makes no sandbox, the switch says what happens: commands run without one. A folder that is
    // not trusted keeps its sandbox, whatever was chosen for it.
    this._refs.switch.checked.set(available && (untrusted || project.sandboxOff !== true));
    this._refs.switch.disabled.set(!available || untrusted);
    this._refs.switch.description.set(untrusted ? SANDBOX_NOTES.kept : SANDBOX_NOTES.free);
    this._refs.problem.description.set(view?.problem ?? '');
    this._refs.problem.hidden = view === undefined || available || view.problem === undefined;
  }

  private async ask(projectId: string): Promise<void> {
    const view = await readSandbox(projectId);
    if (this._askedFor !== projectId) {
      return;
    }
    this._view = view ?? { available: false, problem: UNREACHABLE };
    this.render();
  }
}

ScProjectSandbox.register();
