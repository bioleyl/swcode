/** The browser's offer to install the app, which a page keeps to show when the person asks. */
export interface InstallPromptEvent extends Event {
  prompt(): Promise<unknown>;
}

/**
 * Where installing swcode on this device stands: done; offered by the browser, a press away; to do from the
 * browser's own menu; or not possible, the page being served without HTTPS.
 */
export type InstallState = 'installed' | 'offered' | 'manual' | 'insecure';
