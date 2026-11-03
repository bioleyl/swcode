import type { ServerMessage } from '../../../shared/protocol/messages.schema';

/** A folder as the daemon listed it: its subfolders by name, the way up, or why it cannot be read. */
export type FolderView = Omit<Extract<ServerMessage, { type: 'folders' }>, 'type'>;

/** How a project is made: of a folder that exists, or of a new folder made inside one. */
export type ProjectSource = 'existing' | 'fresh';

/** What the dialog says and offers for each way of making a project. */
export interface SourceView {
  lead: string;
  confirm: (folder: string, name: string) => string;
}
