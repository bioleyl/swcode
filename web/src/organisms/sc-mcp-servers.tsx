import { createElement, css } from '@jadis/core';
import { JlAlert, JlBadge, JlButton, JlListItem } from '@syware/jlib';

import { ScElement } from '../base/sc-element';
import { anyStarting, EXAMPLE_FILE, SERVER_STATES, serverLine } from '../lib/mcp-view';
import { readServers, reloadServers } from '../services/mcp';
import { BASE_STYLES } from '../styles/shared';
import styles from './sc-mcp-servers.css?inline';

import type { McpServer, McpStatus } from '../../../shared/protocol/settings.schema';

const UNREACHABLE = 'The daemon did not answer.';
const NO_SERVERS = 'No MCP server is listed yet. A file such as this one lists two:';
// How long before the list is read again while a server is still being reached.
const STARTING_POLL_MS = 1500;

/**
 * The settings' MCP servers: the servers of the daemon's servers file, whose tools the agent of a coding session has
 * beside its own, where each stands, and a button that has the daemon read the file again. The file is edited by
 * hand: its entries are the ones a server's documentation gives. It reads the list while its topic is shown.
 */
export class ScMcpServers extends ScElement {
  static readonly selector = 'sc-mcp-servers';

  /** Whether its topic is shown. */
  readonly active = this.useChange(false, (active) => (active ? void this.load() : this.stopPolling()));

  private _poll: ReturnType<typeof setTimeout> | undefined;

  private readonly _refs = this.useRefs((ref) => ({
    example: ref('.example'),
    feedback: ref(JlAlert),
    file: ref('.file'),
    list: ref('.list'),
    none: ref('.none'),
    reload: ref<JlButton>('.reload'),
  }));

  templateHtml(): Node {
    return (
      <div class="servers">
        <p class="lead">
          The agent of a coding session has the tools of these servers beside its own, and asks before it calls
          one. They are listed in a file on the daemon's computer, where their programs run too:
        </p>
        <p class="file" />
        <JlAlert tone="danger" hidden />
        <div class="list" />
        <p class="none" hidden>
          {NO_SERVERS}
        </p>
        <pre class="example" hidden>
          {EXAMPLE_FILE}
        </pre>
        <div class="row">
          <JlButton class="reload" icon="refresh" label="Read the file again" />
        </div>
      </div>
    );
  }

  templateCss(): string {
    return css`${BASE_STYLES}${styles}`;
  }

  onConnect(): void {
    this._refs.reload.events.register('press', () => void this.reload());
    if (this.active.get()) {
      void this.load();
    }
  }

  onDisconnect(): void {
    this.stopPolling();
  }

  private async load(): Promise<void> {
    this.render(await readServers());
  }

  private async reload(): Promise<void> {
    this._refs.reload.loading.set(true);
    const status = await reloadServers();
    this._refs.reload.loading.set(false);
    this.render(status);
  }

  private stopPolling(): void {
    clearTimeout(this._poll);
    this._poll = undefined;
  }

  /** The servers and where each stands; read again shortly while one is still being reached. */
  private render(status: McpStatus | undefined): void {
    this.stopPolling();
    if (!this.isConnected) {
      return;
    }
    const problem = status === undefined ? UNREACHABLE : (status.problem ?? '');
    this._refs.feedback.description.set(problem);
    this._refs.feedback.hidden = problem === '';
    if (status === undefined) {
      return;
    }
    const empty = status.servers.length === 0;
    this._refs.file.textContent = status.file;
    this._refs.list.replaceChildren(...status.servers.map((server) => this.serverNode(server)));
    this._refs.none.hidden = !empty;
    this._refs.example.hidden = !empty;
    if (this.active.get() && anyStarting(status.servers)) {
      this._poll = setTimeout(() => void this.load(), STARTING_POLL_MS);
    }
  }

  private serverNode(server: McpServer): Node {
    const state = SERVER_STATES[server.state];
    const row = createElement(JlListItem, {
      attrs: { class: 'server' },
      props: { description: serverLine(server), heading: server.name, icon: 'plug' },
    });
    row.append(
      createElement(JlBadge, { attrs: { slot: 'end' }, props: { label: state.label, tone: state.tone } })
    );
    return row;
  }
}

ScMcpServers.register();
