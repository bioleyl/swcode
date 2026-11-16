import { z } from 'zod';

/** How many digits a pairing number has: the app that pairs shows them, and the person types them on the computer. */
export const PAIR_NUMBER_DIGITS = 6;

const PAIR_NUMBER = new RegExp(`^\\d{${PAIR_NUMBER_DIGITS}}$`);

/**
 * `POST /api/pairing`: the number an app shows, traded once for a token of its own, and what the app calls itself.
 * Open to anyone, the number being the secret: `swcode pair` on the computer has already made it wait.
 */
export const PairRequestSchema = z.object({
  name: z.string().trim().max(60).optional(),
  number: z.string().regex(PAIR_NUMBER, `is ${PAIR_NUMBER_DIGITS} digits`),
});
export type PairRequest = z.infer<typeof PairRequestSchema>;

export const PairAnswerSchema = z.object({ token: z.string() });

/**
 * `POST /api/pairing/pending`: the number `swcode pair <number>` typed here, which the phone's app then redeems. Behind
 * the token, so only this computer's person can make one wait.
 */
export const PairPendingRequestSchema = z.object({
  number: z.string().regex(PAIR_NUMBER, `is ${PAIR_NUMBER_DIGITS} digits`),
});
export type PairPendingRequest = z.infer<typeof PairPendingRequestSchema>;

/** What `swcode pair` gets back: until when the number waits for the app to redeem it. */
export const PairPendingSchema = z.object({
  expiresAt: z.iso.datetime(),
  lifetimeMs: z.number().int().positive(),
});
export type PairPending = z.infer<typeof PairPendingSchema>;

/** A paired phone, as the phone dialog lists it: its token is the phone's alone. */
export const DeviceSchema = z.object({
  id: z.string(),
  name: z.string(),
  pairedAt: z.iso.datetime(),
});
export type Device = z.infer<typeof DeviceSchema>;

export const DevicesSchema = z.object({ devices: z.array(DeviceSchema) });

/**
 * `GET /api/tailnet`, and `POST` to have Tailscale serve swcode there: whether this computer's tailnet can give
 * swcode an HTTPS address, the address (served already, or the one it would be), and why not otherwise. A phone
 * installs swcode as an app only from HTTPS.
 */
export const TailnetSchema = z.object({
  address: z.string().optional(),
  available: z.boolean(),
  problem: z.string().optional(),
  served: z.boolean(),
});
export type Tailnet = z.infer<typeof TailnetSchema>;

/** `GET /api/push`: the daemon's public key, which a browser subscribes to its push service with. */
export const PushKeySchema = z.object({ key: z.string() });
export type PushKey = z.infer<typeof PushKeySchema>;

/**
 * `POST /api/push`: a browser that asks to be told when a session waits for the person or ends its turn, the app
 * closed. What its `PushSubscription` holds: where its push service takes messages for it, and its keys.
 */
export const PushSubscriptionSchema = z.object({
  endpoint: z.string().min(1),
  keys: z.object({ auth: z.string().min(1), p256dh: z.string().min(1) }),
});
export type PushSubscription = z.infer<typeof PushSubscriptionSchema>;

/** `DELETE /api/push`: the browser asks out, by the address of its subscription. */
export const PushRemovalSchema = z.object({ endpoint: z.string().min(1) });
export type PushRemoval = z.infer<typeof PushRemovalSchema>;

/** What a push says, as the service worker reads it: the session's title, what it needs, and the session to open. */
export const PushNoticeSchema = z.object({ body: z.string(), sessionId: z.string(), title: z.string() });
export type PushNotice = z.infer<typeof PushNoticeSchema>;
