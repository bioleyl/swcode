import { Bus } from '@jadis/core';

import { ServerMessageSchema } from '../../../shared/protocol/messages.schema';
import { socketUrl } from './access';

import type { ClientMessage, ServerMessage } from '../../../shared/protocol/messages.schema';

// Waits between attempts to reconnect, longer each time, then the last one again and again.
const RETRY_DELAYS_MS = [500, 1000, 2000, 5000, 10_000];
const LAST_DELAY_MS = 10_000;

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

  get online(): boolean {
    return this._socket?.readyState === WebSocket.OPEN;
  }

  connect(): void {
    const socket = new WebSocket(socketUrl());
    this._socket = socket;
    socket.addEventListener('open', () => this.opened(socket));
    socket.addEventListener('message', (event) => this.received(event.data));
    socket.addEventListener('close', () => this.closed());
  }

  /**
   * Connects now rather than at the end of the wait: the window shows again, or the network is back. A phone that
   * wakes would otherwise show its last page for up to ten seconds before it hears the daemon again.
   */
  retryNow(): void {
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
    for (const message of this._waiting.splice(0)) {
      socket.send(JSON.stringify(message));
    }
    daemonBus.emit('online', true);
  }

  private received(data: unknown): void {
    const message = parse(data);
    if (!message) {
      return;
    }
    daemonBus.emit('message', message);
  }

  private closed(): void {
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
