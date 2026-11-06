import { css } from '@jadis/core';
import { JlBadge, JlListItem } from '@syware/jlib';

import { ScElement } from '../base/sc-element';
import { ago } from '../lib/format';
import { sessionHost } from '../lib/session-kinds';
import { SESSION_STATUS_LOOKS } from '../lib/session-status';
import { hrefOf, sessionPath } from '../services/navigation';
import { BASE_STYLES } from '../styles/shared';
import styles from './sc-session-row.css?inline';

import type { SessionSummary } from '../../../shared/protocol/session.schema';

/** A session in a list: its title, where and when it last worked, its state; the row opens it. */
export class ScSessionRow extends ScElement {
  static readonly selector = 'sc-session-row';

  readonly session = this.useChange<SessionSummary | undefined>(undefined, (session) => this.render(session));

  private readonly _refs = this.useRefs((ref) => ({ badge: ref(JlBadge), row: ref(JlListItem) }));

  templateHtml(): Node {
    return (
      <JlListItem icon="message">
        <JlBadge slot="end" />
      </JlListItem>
    );
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
    this._refs.badge.showStatus(SESSION_STATUS_LOOKS[session.status]);
  }
}

ScSessionRow.register();
