import { DesktopAnswerSchema } from '../../../shared/protocol/desktop.schema';

import type { DesktopAnswer, DesktopRequest } from '../../../shared/protocol/desktop.schema';

const parsedJson = (text: string): unknown => {
  try {
    return JSON.parse(text);
  } catch {
    return undefined;
  }
};

/** Whether the page is in the desktop app's window, which it can then ask things. */
export const inDesktopApp = (): boolean => window.chrome?.webview !== undefined;

/** Asks the desktop app; nothing happens outside its window. */
export function askDesktop(request: DesktopRequest): void {
  window.chrome?.webview?.postMessage(JSON.stringify(request));
}

/**
 * Hears the desktop app's answers until the signal aborts. WebView2 sends the page's own messages back to it too:
 * those are no answers, and are left out.
 */
export function onDesktopAnswer(listener: (answer: DesktopAnswer) => void, signal: AbortSignal): void {
  const webview = window.chrome?.webview;
  if (!webview) {
    return;
  }
  const hear = (event: MessageEvent): void => {
    const parsed = DesktopAnswerSchema.safeParse(
      typeof event.data === 'string' ? parsedJson(event.data) : undefined
    );
    if (parsed.success) {
      listener(parsed.data);
    }
  };
  webview.addEventListener('message', hear);
  signal.addEventListener('abort', () => webview.removeEventListener('message', hear), { once: true });
}

// An app that does not answer leaves notifications on, which is where its tray starts.
const ANSWER_MS = 2000;
const ON = true;

/**
 * Asks the app where its notifications stand, or sets them and hears what it kept: the answer comes back as the
 * page's own message, so one listener waits for it and stops.
 */
export function askNotifications(on?: boolean): Promise<boolean> {
  const asking = new AbortController();
  return new Promise((resolve) => {
    const hear = (answer: DesktopAnswer): void => {
      if (answer.type !== 'notifications') {
        return;
      }
      asking.abort();
      resolve(answer.on);
    };
    onDesktopAnswer(hear, asking.signal);
    askDesktop(on === undefined ? { type: 'notifications' } : { on, type: 'setNotifications' });
    setTimeout(() => {
      if (!asking.signal.aborted) {
        asking.abort();
        resolve(ON);
      }
    }, ANSWER_MS);
  });
}
