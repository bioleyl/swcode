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
