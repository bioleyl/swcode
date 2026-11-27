import { hostLabel } from '../../../shared/hosts/host-key';
import { LOCAL_HOST } from '../../../shared/protocol/project.schema';
import { daemon, daemonBus } from './daemon';

import type { ClientMessage, ServerMessage } from '../../../shared/protocol/messages.schema';
import type { HostOption, HostRef, Project } from '../../../shared/protocol/project.schema';
import type { Answer } from '../types/answer.types';
import type { ServerMessageMap } from '../types/page.types';

type Reply = Answer<ServerMessage>;

// The hosts the daemon lists when it knows only the computer it runs on.
const LOCAL_ONLY: HostOption = { label: hostLabel(LOCAL_HOST), ref: LOCAL_HOST };

// How long an answer is waited for: listing a folder over a slow SSH link takes seconds, not minutes.
const ANSWER_WAIT_MS = 60_000;
const NO_ANSWER = 'The daemon did not answer.';

// The answers live as long as the page: the listener that delivers them never needs to stop.
const forever = new AbortController().signal;

/**
 * One request to the daemon and the answer it sends back. The daemon's replies are a broadcast, so the first answer
 * of the kind asked for ends the request, and the reply that refuses that kind ends it as a refusal: the caller no
 * longer has to tell whose reply this was.
 */
class PendingAnswers {
  private readonly _waiting = new Map<keyof ServerMessageMap, (reply: Reply) => void>();
  // The last request of each kind: the next of that kind is sent once it was answered.
  private readonly _asked = new Map<keyof ServerMessageMap, Promise<unknown>>();

  constructor() {
    daemonBus.register('message', (message) => this.deliver(message), forever);
  }

  /**
   * Sends the request and waits for the answer of the kind asked for, or for a `problem` saying why not. Two
   * requests for the same kind go one after the other: answers carry no word of which request they are for, so one
   * sent while another waits could take the other's answer (a slow host's folders, shown under the next host).
   */
  ask<Kind extends keyof ServerMessageMap>(
    request: ClientMessage,
    answer: Kind
  ): Promise<Answer<ServerMessageMap[Kind]>> {
    const before = this._asked.get(answer) ?? Promise.resolve();
    const asked = before.then(
      () =>
        new Promise<Answer<ServerMessageMap[Kind]>>((resolve) => {
          // A daemon that never answers (its socket went) must not hold up the requests after this one.
          const late = setTimeout(() => this.settle(answer, { ok: false, problem: NO_ANSWER }), ANSWER_WAIT_MS);
          // The reply delivered to this slot is the one its kind was asked for, which the map cannot show.
          this._waiting.set(answer, (reply) => {
            clearTimeout(late);
            resolve(reply as Answer<ServerMessageMap[Kind]>);
          });
          daemon.send(request);
        })
    );
    this._asked.set(answer, asked);
    return asked;
  }

  private deliver(message: ServerMessage): void {
    if (message.type === 'projectRefused') {
      this.settle('projectCreated', { ok: false, problem: message.message });
      return;
    }
    this.settle(message.type, { answer: message, ok: true });
  }

  /** Ends the request waiting for that kind of answer, if one is. */
  private settle(kind: keyof ServerMessageMap, reply: Reply): void {
    const waiting = this._waiting.get(kind);
    this._waiting.delete(kind);
    waiting?.(reply);
  }
}

const answers = new PendingAnswers();

/** The hosts a project can live on: this computer, its WSL distributions, the SSH endpoints. */
export const askHosts = (): Promise<Answer<HostOption[]>> =>
  answers
    .ask({ type: 'listHosts' }, 'hosts')
    .then((reply) =>
      reply.ok ? { answer: reply.answer.hosts.length === 0 ? [LOCAL_ONLY] : reply.answer.hosts, ok: true } : reply
    );

/**
 * A folder's subfolders, for the project dialog: without a path, of the folder projects usually go in. The answer's
 * own `problem` says when the folder cannot be read; this one when the daemon refused the request.
 */
export const askFolders = (host: HostRef, path?: string): Promise<Answer<ServerMessageMap['folders']>> =>
  answers.ask({ host, ...(path === undefined ? {} : { path }), type: 'listFolders' }, 'folders');

/** Makes a project of a folder, or of a new folder made there: refused, it says why. */
export const askCreateProject = (folder: string, host: HostRef, newFolder?: string): Promise<Answer<Project>> =>
  answers
    .ask(
      { folder, host, ...(newFolder === undefined ? {} : { newFolder }), type: 'createProject' },
      'projectCreated'
    )
    .then((reply) => (reply.ok ? { answer: reply.answer.project, ok: true } : reply));
