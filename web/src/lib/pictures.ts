import type { PictureUpload } from '../../../shared/protocol/picture.schema';
import type { Answer } from '../types/answer.types';

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
      (blob) => (blob ? resolve(blob) : reject(new PictureProblem('The browser could not make the JPEG.'))),
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

/** A picture the browser could not turn into one the model sees, with what to tell the person. */
class PictureProblem extends Error {
  constructor(readonly problem: string) {
    super(problem);
  }
}

const UNREADABLE = 'That picture could not be read: paste a PNG, a JPEG or a screenshot.';

/** Why a picture did not become one the model can see: the person's mistake, or the browser refusing. */
const problemOf = (error: unknown): string =>
  error instanceof PictureProblem ? error.problem : `The picture could not be made: ${String(error)}`;

/** The picture drawn on white and made a JPEG of; the browser refusing either step is a PictureProblem. */
const jpegOfBitmap = async (bitmap: ImageBitmap): Promise<PictureUpload> => {
  const size = fittedSize(bitmap.width, bitmap.height);
  const canvas = document.createElement('canvas');
  canvas.width = size.width;
  canvas.height = size.height;
  const context = canvas.getContext('2d');
  if (!context) {
    throw new PictureProblem('The browser could not draw the picture.');
  }
  context.fillStyle = BACKGROUND;
  context.fillRect(0, 0, size.width, size.height);
  context.drawImage(bitmap, 0, 0, size.width, size.height);
  return { data: await base64Of(await jpegOf(canvas)), height: size.height, type: JPEG, width: size.width };
};

/**
 * A pasted picture made small enough for the model, as a JPEG: llm-server reads JPEG and PNG, and a JPEG of a
 * screenshot is a fraction of its PNG. Drawn on white, so a transparent PNG does not turn black. A picture the
 * browser cannot decode, or cannot draw, comes back as a problem the composer can show.
 */
export async function shrinkPicture(file: Blob): Promise<Answer<PictureUpload>> {
  let bitmap: ImageBitmap;
  try {
    bitmap = await createImageBitmap(file);
  } catch {
    return { ok: false, problem: UNREADABLE };
  }
  try {
    return { answer: await jpegOfBitmap(bitmap), ok: true };
  } catch (error) {
    return { ok: false, problem: problemOf(error) };
  } finally {
    bitmap.close();
  }
}
