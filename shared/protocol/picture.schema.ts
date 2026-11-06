import { z } from 'zod';

/** What llm-server reads (stb_image: no WebP), and so what the app sends. */
export const PICTURE_TYPES = ['image/jpeg', 'image/png'] as const;
export const PictureTypeSchema = z.enum(PICTURE_TYPES);
export type PictureType = z.infer<typeof PictureTypeSchema>;

/** The pictures a message may carry: each takes a thousand tokens or so of the model's context. */
export const MAX_PICTURES = 4;
// The app sends a picture made small, 1280 px on its longest side: a few hundred kB as base64.
const MAX_PICTURE_BASE64 = 8_000_000;
const BASE64 = /^[A-Za-z0-9+/]+={0,2}$/;

const side = z.number().int().positive();

/** A picture the person pasted, as the app sends it: its bytes in base64, and its size in pixels. */
export const PictureUploadSchema = z.object({
  data: z.string().max(MAX_PICTURE_BASE64).regex(BASE64),
  height: side,
  type: PictureTypeSchema,
  width: side,
});
export type PictureUpload = z.infer<typeof PictureUploadSchema>;

/** A picture of a message as the session keeps it: a file among the session's pictures, and its size. */
export const PictureRefSchema = z.object({ file: z.string(), height: side, type: PictureTypeSchema, width: side });
export type PictureRef = z.infer<typeof PictureRefSchema>;
