/**
 * Where notifications stand on the device that shows the page: on, off and ready to be turned on, or why they cannot
 * be — a page without HTTPS, an iPhone whose swcode is not on its Home Screen yet, a person who blocked them, a
 * browser that has none.
 */
export type PushState = 'denied' | 'insecure' | 'needsInstall' | 'off' | 'on' | 'unsupported';

/** How turning notifications on or off went: done, or why not. */
export type PushOutcome = { ok: true } | { ok: false; problem: string };
