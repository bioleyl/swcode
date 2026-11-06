import { css } from '@jadis/core';
import { JlBadge, JlButton, JlListItem } from '@syware/jlib';

import { ScElement } from '../base/sc-element';
import { ago } from '../lib/format';
import { sessionHost } from '../lib/session-kinds';
import { liveStatus, SESSION_STATUS_LOOKS } from '../lib/session-status';
import { hrefOf, sessionPath } from '../services/navigation';
import { BASE_STYLES } from '../styles/shared';
import styles from './sc-session-row.css?inline';

import type { SessionSummary } from '../../../shared/protocol/session.schema';

const DELETE_HINT = 'Delete this session';
const WORKING_HINT = 'Stop this session before deleting it';

/**
 * A session in a list: its title, where and when it last worked, its state; the row opens it. Where the list tidies
 * them (a project's page), a button at its end deletes it, once the session no longer works.
 */
export class ScSessionRow extends ScElement {
  static readonly selector = 'sc-session-row';

  /** `remove`: its delete button pressed; the list asks first. */
  readonly events = this.useEvents<{ remove: SessionSummary }>();

  readonly session = this.useChange<SessionSummary | undefined>(undefined, (session) => this.render(session));
  /** Whether the row has its delete button. */
  readonly removable = this.useChange(false, () => this.render(this.session.get()));

  private readonly _refs = this.useRefs((ref) => ({
    badge: ref(JlBadge),
    remove: ref(JlButton),
    row: ref(JlListItem),
  }));

  templateHtml(): Node {
    return (
      <JlListItem icon="message">
        <JlBadge slot="end" />
        <JlButton slot="end" icon="trash" variant="ghost" size="sm" hint={DELETE_HINT} hidden />
      </JlListItem>
    );
  }

  onConnect(): void {
    this._refs.remove.events.register('press', () => {
      const session = this.session.get();
      if (!session) {
        return;
      }
      this.events.emit('remove', session);
    });
  }

  templateCss(): string {
    return css`${BASE_STYLES}${styles}`;
  }

  private render(session: SessionSummary | undefined): void {
    if (!session) {
      return;
    }
    this._refs.row.heading.set(session.title);
    this._refs.row.description.set(`${sessionHost(session)} · ${ago(session.updatedAt, new Date())}`);
    this._refs.row.href.set(hrefOf(sessionPath(session.id)));
    const status = liveStatus(session);
    this._refs.badge.showStatus(SESSION_STATUS_LOOKS[status]);
    // The daemon deletes no session at work.
    const remove = this._refs.remove;
    remove.hidden = !this.removable.get();
    remove.disabled.set(status !== 'idle');
    remove.hint.set(status === 'idle' ? DELETE_HINT : WORKING_HINT);
  }
}

ScSessionRow.register();
