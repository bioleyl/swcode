import { createElement, css } from '@jadis/core';
import { JlButton, JlEmpty, JlPage } from '@syware/jlib';

import { SESSION_STATUSES } from '../../../shared/protocol/session.schema';
import { ScElement } from '../base/sc-element';
import { liveStatus } from '../lib/session-status';
import { ScSessionRow } from '../molecules/sc-session-row';
import { hrefOf, NEW_SESSION_PATH } from '../services/navigation';
import { allSessions, sessionListBus } from '../services/session-list';
import { BASE_STYLES } from '../styles/shared';
import styles from './sessions-page.css?inline';

import type { SessionStatus, SessionSummary } from '../../../shared/protocol/session.schema';

const APP_NAME = 'swcode';

/** The groups of the list, what needs the person first: a new state does not compile until it has its place. */
const GROUP_ORDER: Readonly<Record<SessionStatus, number>> = { idle: 2, waiting: 0, working: 1 };
const GROUP_HEADINGS: Readonly<Record<SessionStatus, string>> = {
  idle: 'Recent',
  waiting: 'Needs you',
  working: 'Working',
};

/** Every session on the daemon, those that need the person first, then those at work, then the others. */
export class SessionsPage extends ScElement {
  static readonly selector = 'sessions-page';

  private readonly _refs = this.useRefs((ref) => ({ empty: ref(JlEmpty), groups: ref('.groups') }));

  templateHtml(): Node {
    return (
      <JlPage
        heading="Sessions"
        description="What swcode works on: what needs you comes first."
        appName={APP_NAME}>
        <JlButton
          slot="actions"
          icon="plus"
          variant="primary"
          label="New session"
          href={hrefOf(NEW_SESSION_PATH)}
        />
        <div class="groups" />
        <JlEmpty icon="message" heading="No sessions yet" description="Start one: tell swcode what to work on.">
          <JlButton variant="primary" icon="plus" label="New session" href={hrefOf(NEW_SESSION_PATH)} />
        </JlEmpty>
      </JlPage>
    );
  }

  templateCss(): string {
    return css`${BASE_STYLES}${styles}`;
  }

  onConnect(): void {
    this.onBus(sessionListBus, 'changed', () => this.render());
    this.render();
  }

  private render(): void {
    const sessions = allSessions();
    const statuses = [...SESSION_STATUSES].sort((first, second) => GROUP_ORDER[first] - GROUP_ORDER[second]);
    this._refs.groups.replaceChildren(
      ...statuses.flatMap((status) =>
        this.groupNodes(
          status,
          sessions.filter((session) => liveStatus(session) === status)
        )
      )
    );
    this._refs.empty.hidden = sessions.length > 0;
  }

  private groupNodes(status: SessionStatus, sessions: readonly SessionSummary[]): Node[] {
    if (sessions.length === 0) {
      return [];
    }
    return [
      <section>
        <h2>{GROUP_HEADINGS[status]}</h2>
        {sessions.map((session) => createElement(ScSessionRow, { props: { session } }))}
      </section>,
    ];
  }
}

SessionsPage.register();
