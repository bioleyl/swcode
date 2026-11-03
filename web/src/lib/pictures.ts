import type { PictureUpload } from '../../../shared/protocol/picture.schema';

// A Qwen vision encoder takes a token for each 32 × 32 pixels: a screenshot this size costs about 900 tokens, a
// square picture 1,600, where a 4K screenshot would take 8,000.
export const MAX_PICTURE_SIDE = 1280;
const JPEG = 'image/jpeg';
// Text in a screenshot stays sharp at this quality, for a few hundred kB.
const JPEG_QUALITY = 0.9;
const BACKGROUND = '#ffffff';
const IMAGE_PREFIX = 'image/';
const DATA_SEPARATOR = ',';

/** A picture's size once its longest side fits; a smaller one keeps its own. */
export function fittedSize(
  width: number,
  height: number,
  maxSide = MAX_PICTURE_SIDE
): { width: number; height: number } {
  const scale = Math.min(1, maxSide / Math.max(width, height));
  return { height: Math.max(1, Math.round(height * scale)), width: Math.max(1, Math.round(width * scale)) };
}

export const isPicture = (file: File): boolean => file.type.startsWith(IMAGE_PREFIX);

/** How the app shows a picture it holds: inline, from its bytes. */
export const pictureUrl = (picture: PictureUpload): string => `data:${picture.type};base64,${picture.data}`;

const jpegOf = (canvas: HTMLCanvasElement): Promise<Blob> =>
  new Promise((resolve, reject) =>
    canvas.toBlob(
      (blob) => (blob ? resolve(blob) : reject(new Error('The picture could not be made.'))),
      JPEG,
      JPEG_QUALITY
    )
  );

const base64Of = (blob: Blob): Promise<string> =>
  new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => {
      const url = String(reader.result);
      resolve(url.slice(url.indexOf(DATA_SEPARATOR) + 1));
    };
    reader.onerror = () => reject(reader.error);
    reader.readAsDataURL(blob);
  });

/**
 * A pasted picture made small enough for the model, as a JPEG: llama-server reads JPEG and PNG, and a JPEG of a
 * screenshot is a fraction of its PNG. Drawn on white, so a transparent PNG does not turn black.
 */
export async function shrinkPicture(file: Blob): Promise<PictureUpload> {
  const bitmap = await createImageBitmap(file);
  const size = fittedSize(bitmap.width, bitmap.height);
  const canvas = document.createElement('canvas');
  canvas.width = size.width;
  canvas.height = size.height;
  const context = canvas.getContext('2d');
  if (!context) {
    bitmap.close();
    throw new Error('The picture could not be drawn.');
  }
  context.fillStyle = BACKGROUND;
  context.fillRect(0, 0, size.width, size.height);
  context.drawImage(bitmap, 0, 0, size.width, size.height);
  bitmap.close();
  return { data: await base64Of(await jpegOf(canvas)), height: size.height, type: JPEG, width: size.width };
}
