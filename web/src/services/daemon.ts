import { Bus } from '@jadis/core';

import { ServerMessageSchema } from '../../../shared/protocol/messages.schema';
import { socketUrl } from './access';

import type { ClientMessage, ServerMessage } from '../../../shared/protocol/messages.schema';

// Waits between attempts to reconnect, longer each time, then the last one again and again.
const RETRY_DELAYS_MS = [500, 1000, 2000, 5000, 10_000];
const LAST_DELAY_MS = 10_000;
// How often the page asks whether the daemon still hears it, and how long any word back may take: a socket can go
// without either end being told, a phone's above all, and would show as open for good.
const PING_EVERY_MS = 25_000;
const PONG_WAIT_MS = 10_000;
// A window that shows again asks at once, and waits less: its socket may have gone while it slept.
const WAKE_WAIT_MS = 3000;

/** Everything the daemon says, and whether the socket is open: on reconnecting, pages ask again what they show. */
export const daemonBus = new Bus<{ message: ServerMessage; online: boolean }>();

const parse = (data: unknown): ServerMessage | undefined => {
  try {
    const parsed = ServerMessageSchema.safeParse(JSON.parse(String(data)));
    return parsed.success ? parsed.data : undefined;
  } catch {
    return undefined;
  }
};

/** The one socket to the daemon: messages sent while it is closed wait for it to open again. */
class DaemonConnection {
  private _socket: WebSocket | undefined;
  private _waiting: ClientMessage[] = [];
  private _attempts = 0;
  // The wait before the next attempt to connect, while the socket is closed.
  private _retry: ReturnType<typeof setTimeout> | undefined;
  // The pings of the open socket, and the wait for a word back after one.
  private _pings: ReturnType<typeof setInterval> | undefined;
  private _deaf: ReturnType<typeof setTimeout> | undefined;

  get online(): boolean {
    return this._socket?.readyState === WebSocket.OPEN;
  }

  connect(): void {
    const socket = new WebSocket(socketUrl());
    this._socket = socket;
    socket.addEventListener('open', () => this.opened(socket));
    socket.addEventListener('message', (event) => this.received(event.data));
    socket.addEventListener('close', () => this.closed(socket));
  }

  /**
   * The window shows again, or the network is back: a closed socket is tried at once rather than at the end of its
   * wait, and an open one is asked whether the daemon still hears it.
   */
  wake(): void {
    if (this.online && this._socket) {
      this.ask(this._socket, WAKE_WAIT_MS);
      return;
    }
    this.retryNow();
  }

  /** Connects now rather than at the end of the wait. */
  private retryNow(): void {
    if (this._retry === undefined) {
      return;
    }
    clearTimeout(this._retry);
    this._retry = undefined;
    this.connect();
  }

  send(message: ClientMessage): void {
    if (!this.online) {
      this._waiting.push(message);
      return;
    }
    this._socket?.send(JSON.stringify(message));
  }

  private opened(socket: WebSocket): void {
    this._attempts = 0;
    // A page takes no session's stream until it opens one: a socket that said nothing would be sent them all.
    socket.send(JSON.stringify({ type: 'closeSession' } satisfies ClientMessage));
    for (const message of this._waiting.splice(0)) {
      socket.send(JSON.stringify(message));
    }
    this._pings = setInterval(() => this.ask(socket, PONG_WAIT_MS), PING_EVERY_MS);
    daemonBus.emit('online', true);
  }

  /** Asks the daemon whether it still hears this socket: without a word back in time, the socket is gone. */
  private ask(socket: WebSocket, waitMs: number): void {
    clearTimeout(this._deaf);
    socket.send(JSON.stringify({ type: 'ping' } satisfies ClientMessage));
    this._deaf = setTimeout(() => this.lost(socket), waitMs);
  }

  /** A socket that hears nothing is closed here and now: its own closing could take as long as it took to notice. */
  private lost(socket: WebSocket): void {
    socket.close();
    this.closed(socket);
  }

  private received(data: unknown): void {
    // Any word from the daemon says the socket lives.
    clearTimeout(this._deaf);
    const message = parse(data);
    if (!message) {
      return;
    }
    daemonBus.emit('message', message);
  }

  private closed(socket: WebSocket): void {
    // A socket given up as lost closes for real later: it is no longer this one.
    if (socket !== this._socket) {
      return;
    }
    this._socket = undefined;
    clearInterval(this._pings);
    clearTimeout(this._deaf);
    daemonBus.emit('online', false);
    const delay = RETRY_DELAYS_MS[this._attempts] ?? LAST_DELAY_MS;
    this._attempts += 1;
    this._retry = setTimeout(() => {
      this._retry = undefined;
      this.connect();
    }, delay);
  }
}

export const daemon = new DaemonConnection();
