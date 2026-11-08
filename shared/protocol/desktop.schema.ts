import { z } from 'zod';

/**
 * The page in the desktop app's window talks to the app itself through WebView2's messages, JSON strings both ways:
 * the computers the app shows (this one, and the others paired with it), and links that open in the browser. A
 * computer's token stays with the app: no page ever sees it.
 */
export const THIS_COMPUTER = 'local';

/** The colours a paired computer can take, as the app's primary colour while it shows that computer. */
export const COMPUTER_COLORS = [
  'amber',
  'blue',
  'green',
  'orange',
  'pink',
  'red',
  'slate',
  'teal',
  'violet',
] as const;
export const ComputerColorSchema = z.enum(COMPUTER_COLORS);
export type ComputerColor = z.infer<typeof ComputerColorSchema>;

// `reachable`: its swcode answers the app now. `color`: the primary colour it takes while shown; none for the default.
export const ComputerSchema = z.object({
  address: z.string(),
  color: ComputerColorSchema.optional(),
  id: z.string(),
  name: z.string(),
  reachable: z.boolean(),
});
export type Computer = z.infer<typeof ComputerSchema>;

/** What the page asks the app. */
export const DesktopRequestSchema = z.discriminatedUnion('type', [
  z.object({ type: z.literal('listComputers') }),
  // An address and the code another computer's swcode shows to pair with it; a link with its token needs no code.
  z.object({
    address: z.string().trim().min(1).max(300),
    code: z.string().trim().max(100).optional(),
    type: z.literal('addComputer'),
  }),
  z.object({ id: z.string(), type: z.literal('openComputer') }),
  // The name and colour the app keeps for that computer; no colour is the default one.
  z.object({
    color: ComputerColorSchema.optional(),
    id: z.string(),
    name: z.string().trim().min(1).max(60),
    type: z.literal('editComputer'),
  }),
  z.object({ id: z.string(), type: z.literal('forgetComputer') }),
  z.object({ type: z.literal('openOutside'), url: z.string() }),
]);
export type DesktopRequest = z.infer<typeof DesktopRequestSchema>;

/** What the app answers: the computers after any change, or why one could not be added. */
export const DesktopAnswerSchema = z.discriminatedUnion('type', [
  z.object({ computers: z.array(ComputerSchema), current: z.string(), type: z.literal('computers') }),
  z.object({ message: z.string(), type: z.literal('computerRefused') }),
]);
export type DesktopAnswer = z.infer<typeof DesktopAnswerSchema>;
