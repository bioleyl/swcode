import { z } from 'zod';

/** How swcode starts when its person logs in: not at all, in the tray (the app), or the daemon alone, without icon. */
export const STARTUP_MODES = ['off', 'tray', 'daemon'] as const;
export const StartupModeSchema = z.enum(STARTUP_MODES);
export type StartupMode = z.infer<typeof StartupModeSchema>;

/** `GET /api/startup`: whether this daemon can start with its computer (the Windows app can), and how it does. */
export const StartupViewSchema = z.object({ available: z.boolean(), mode: StartupModeSchema });
export type StartupView = z.infer<typeof StartupViewSchema>;

/** `PUT /api/startup`: how it should from now on. */
export const StartupChangeSchema = z.object({ mode: StartupModeSchema });
export type StartupChange = z.infer<typeof StartupChangeSchema>;
