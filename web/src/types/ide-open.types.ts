/**
 * The file the host (the VS Code extension's webview) is asked to open, with the range of lines to show.
 * The lines are 1-based, as the editor counts them; both are absent when the whole file is meant.
 */
export interface OpenRequest {
  type: 'open';
  /** The file's path, resolved to the project's folder so the host opens it whole. */
  path: string;
  startLine?: number;
  endLine?: number;
}
