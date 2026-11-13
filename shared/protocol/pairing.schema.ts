import { z } from 'zod';

/** `GET /api/pairing/code`: a code a phone trades once for its own token, valid until it expires. */
export const PairingCodeSchema = z.object({
  code: z.string(),
  expiresAt: z.iso.datetime(),
  lifetimeMs: z.number().int().positive(),
});
export type PairingCode = z.infer<typeof PairingCodeSchema>;

/** `POST /api/pairing`: the code from the QR code, and what the phone calls itself. */
export const PairRequestSchema = z.object({
  code: z.string().min(1).max(100),
  name: z.string().trim().max(60).optional(),
});
export type PairRequest = z.infer<typeof PairRequestSchema>;

export const PairAnswerSchema = z.object({ token: z.string() });

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
