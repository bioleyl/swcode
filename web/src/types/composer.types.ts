import type { PictureUpload } from '../../../shared/protocol/picture.schema';

/** What the composer sends: the words, and the pictures pasted with them; either may be empty, not both. */
export interface PromptDraft {
  text: string;
  pictures: PictureUpload[];
}
