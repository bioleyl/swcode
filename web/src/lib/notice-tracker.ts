import type { ServerMessage } from '../../../shared/protocol/messages.schema';
import type { SessionEvent, TurnOutcome } from '../../../shared/protocol/session.schema';

/** What a notice tells: a session waits for its person, ended its turn, or failed. */
export const NOTICE_KINDS = ['needsYou', 'finished', 'failed'] as const;
export type NoticeKind = (typeof NOTICE_KINDS)[number];

/** What a notification says, and the session it opens. */
export type Notice = { kind: NoticeKind; sessionId: string; text: string; title: string };

// A notification shows a line or two.
const MAX_TEXT = 160;
const ELLIPSIS = '…';

const UNTITLED = 'A session';
const ASKED_FIRST = 'Asks you first: ';
const TURN_OVER = 'Its turn is over.';
const TURN_FAILED = 'Its turn failed.';
const inTerminal = (title: string): string =>
  `A command runs in the terminal “${title}”: it may ask for your password there.`;

/** How a turn ends and what each way tells: a turn its person stopped tells nothing. */
const OUTCOMES: Partial<Record<TurnOutcome, NoticeKind>> = {
  done: 'finished',
  failed: 'failed',
  limit: 'finished',
};

/**
 * Turns what a daemon's socket says into notices, in the same words as the desktop app's tray and the daemon's Web
 * Push: it keeps the sessions' titles, their last answers, and their calls, which an approval names.
 */
export class NoticeTracker {
  private _titles = new Map<string, string>();
  private _replies = new Map<string, string>();
  private _calls = new Map<string, Map<string, string>>();
  private _subagents = new Map<string, number>();

  /** One message of the socket: a notice, when it is one. */
  take(message: ServerMessage): Notice | undefined {
    if (message.type === 'sessions') {
      for (const session of message.sessions) {
        this.keep(session.id, session.subagents, session.title);
      }
      return undefined;
    }
    if (message.type === 'sessionCreated') {
      this.keep(message.session.id, message.session.subagents, message.session.title);
      return undefined;
    }
    if (message.type === 'sessionEvent') {
      return this.event(message.sessionId, message.event);
    }
    return undefined;
  }

  private keep(sessionId: string, subagents: number, title: string): void {
    this._titles.set(sessionId, title);
    this._subagents.set(sessionId, subagents);
  }

  private event(sessionId: string, event: SessionEvent): Notice | undefined {
    const notice: Notice = { kind: 'needsYou', sessionId, text: '', title: this.titleOf(sessionId) };
    switch (event.kind) {
      case 'toolCall':
        this.callsOf(sessionId).set(event.callId, event.summary);
        return undefined;
      case 'assistantMessage':
        this.keepReply(sessionId, event.text);
        return undefined;
      case 'permissionRequested':
        return { ...notice, text: clipped(ASKED_FIRST + (this.callsOf(sessionId).get(event.callId) ?? '')) };
      case 'questionAsked':
        return { ...notice, text: clipped(event.questions[0]?.question ?? '') };
      case 'passwordRequested':
        return { ...notice, text: clipped(event.prompt) };
      case 'terminalWaiting':
        return { ...notice, text: clipped(inTerminal(event.title)) };
      case 'turnFinished':
        return this.finished(notice, event.outcome, event.message);
      default:
        return undefined;
    }
  }

  private titleOf(sessionId: string): string {
    return this._titles.get(sessionId) || UNTITLED;
  }

  private callsOf(sessionId: string): Map<string, string> {
    let calls = this._calls.get(sessionId);
    if (!calls) {
      calls = new Map<string, string>();
      this._calls.set(sessionId, calls);
    }
    return calls;
  }

  /** A session's last words to its person: what a finished turn's notice says. */
  private keepReply(sessionId: string, text: string): void {
    if (text.trim() === '') {
      return;
    }
    this._replies.set(sessionId, text);
  }

  /** The notice a turn's end makes, with what was kept of that turn. A turn that ended while subagents work makes
   * none: their reports wake the agent, whose next turn tells. */
  private finished(notice: Notice, outcome: TurnOutcome, message: string | undefined): Notice | undefined {
    const reply = this._replies.get(notice.sessionId);
    this._replies.delete(notice.sessionId);
    this._calls.delete(notice.sessionId);
    const kind = OUTCOMES[outcome];
    if (!kind || (kind === 'finished' && (this._subagents.get(notice.sessionId) ?? 0) > 0)) {
      return undefined;
    }
    return { ...notice, kind, text: clipped(endingText(kind, reply, message)) };
  }
}

function endingText(kind: NoticeKind, reply: string | undefined, problem: string | undefined): string {
  return kind === 'failed' ? firstOf(problem, TURN_FAILED) : firstOf(reply, TURN_OVER);
}

function firstOf(text: string | undefined, otherwise: string): string {
  return text && text.trim() !== '' ? text : otherwise;
}

/** A text on one line, cut to what a notification shows. */
function clipped(text: string): string {
  const line = text.split(/\s+/).join(' ').trim();
  const runes = Array.from(line);
  return runes.length <= MAX_TEXT ? line : `${runes.slice(0, MAX_TEXT - 1).join('')}${ELLIPSIS}`;
}
