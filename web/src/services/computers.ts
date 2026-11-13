import { askBrowser, browserComputersBus } from './browser-computers';
import { askDesktop, inDesktopApp, onDesktopAnswer } from './desktop';

import type { DesktopAnswer } from '../../../shared/protocol/desktop.schema';
import type { ComputerRequest } from '../types/computers.types';

/**
 * Asks about the computers the app shows: this one, and the others paired with it. The desktop app keeps them for
 * its window and moves it from one to the other; anywhere else (a browser, the app a phone installed) the browser
 * keeps them itself and the page talks to the one chosen.
 */
export function askComputers(request: ComputerRequest): void {
  inDesktopApp() ? askDesktop(request) : askBrowser(request);
}

/** Hears the answers about the computers until the signal aborts. */
export function onComputersAnswer(listener: (answer: DesktopAnswer) => void, signal: AbortSignal): void {
  inDesktopApp() ? onDesktopAnswer(listener, signal) : browserComputersBus.register('answer', listener, signal);
}
