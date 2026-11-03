import type { DesktopAnswer } from '../../../shared/protocol/desktop.schema';

/** WebView2's channel between a page and the desktop app around it, as `window.chrome.webview`. */
export interface DesktopWebView {
  postMessage(message: string): void;
  addEventListener(type: 'message', listener: (event: MessageEvent) => void): void;
  removeEventListener(type: 'message', listener: (event: MessageEvent) => void): void;
}

declare global {
  interface Window {
    chrome?: { webview?: DesktopWebView };
  }
}

export type DesktopAnswerMap = { [Answer in DesktopAnswer as Answer['type']]: Answer };

/** One handler per kind of answer from the desktop app: a new kind does not compile until the page decides. */
export type DesktopAnswerHandlers = {
  [Type in keyof DesktopAnswerMap]: (answer: DesktopAnswerMap[Type]) => void;
};
