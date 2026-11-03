import { createElement, css } from '@jadis/core';
import { JlButton, JlEmpty, JlPage } from '@syware/jlib';

import { ScElement } from '../base/sc-element';
import { placeText } from '../lib/hosts';
import { ScSessionRow } from '../molecules/sc-session-row';
import { hrefOf, newSessionPath } from '../services/navigation';
import { askToRemoveProject, findProject, projectListBus } from '../services/project-list';
import { allSessions, sessionListBus } from '../services/session-list';
import { BASE_STYLES } from '../styles/shared';
import styles from './project-page.css?inline';

const APP_NAME = 'swcode';
const NOT_FOUND = 'This project is not on the daemon';

/** A project: where its folder is, its sessions, the newest first, and a new session in it. */
export class ProjectPage extends ScElement {
  static readonly selector = 'project-page';

  readonly route = this.useAttributes({ id: () => this.render() });

  private readonly _refs = this.useRefs((ref) => ({
    empty: ref(JlEmpty),
    list: ref('.sessions'),
    newSession: ref<JlButton>('.new-session'),
    page: ref(JlPage),
    remove: ref<JlButton>('.remove'),
  }));

  templateHtml(): Node {
    return (
      <JlPage appName={APP_NAME} width="standard">
        <JlButton slot="actions" class="remove" icon="trash" variant="ghost" label="Remove" />
        <JlButton slot="actions" class="new-session" icon="plus" variant="primary" label="New session" />
        <div class="sessions" />
        <JlEmpty
          icon="message"
          heading="No sessions yet"
          description="A new session works in this project’s folder."
        />
      </JlPage>
    );
  }

  templateCss(): string {
    return css`${BASE_STYLES}${styles}`;
  }

  onConnect(): void {
    this._refs.remove.events.register('press', () => askToRemoveProject(this.route.id ?? ''));
    this.onBus(projectListBus, 'changed', () => this.render());
    this.onBus(sessionListBus, 'changed', () => this.render());
    this.render();
  }

  private render(): void {
    const project = findProject(this.route.id ?? '');
    this._refs.page.heading.set(project?.name ?? NOT_FOUND);
    this._refs.page.description.set(project ? placeText(project.host, project.path) : '');
    this._refs.newSession.href.set(project ? hrefOf(newSessionPath(project.id)) : '');
    this._refs.newSession.hidden = !project;
    this._refs.remove.hidden = !project;
    const sessions = project ? allSessions().filter((session) => session.workspace === project.path) : [];
    this._refs.list.replaceChildren(
      ...sessions.map((session) => createElement(ScSessionRow, { props: { session } }))
    );
    this._refs.empty.hidden = !project || sessions.length > 0;
  }
}

ProjectPage.register();
