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
