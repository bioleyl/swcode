import type { ToolDisplay, ToolName } from '../../../shared/protocol/session.schema';

/** The tools whose call's few words are a file: their summary names the path the call works on. */
const FILE_TOOLS: readonly ToolName[] = ['edit', 'read', 'write'];

/** A path that stands on its own: from a root, or from a Windows drive. */
const isFullPath = (path: string): boolean =>
  path.startsWith('/') || path.startsWith('\\') || /^[A-Za-z]:[\\/]/.test(path);

/** The separator of a folder: `\` on Windows, `/` everywhere else. */
const separatorOf = (folder: string): string => (folder.includes('\\') ? '\\' : '/');

const trimmedEnd = (folder: string): string => folder.replace(/[/\\]+$/, '');

const trimmedStart = (path: string): string => path.replace(/^[/\\]+/, '');

/**
 * A tool's path, as the editor opens it: a path relative to the project joined onto the project's folder,
 * a path that is already whole (the scratch folder) as it is. The folder's separator is kept, so a project on
 * Windows is opened with `\` and one on any other system with `/`.
 */
export function absolutePath(folder: string, path: string): string {
  if (path === '') {
    return folder;
  }
  if (isFullPath(path)) {
    return path;
  }
  if (folder === '') {
    return path;
  }
  return `${trimmedEnd(folder)}${separatorOf(folder)}${trimmedStart(path)}`;
}

/** The file a call concerns: the change's path, else the path a file tool's summary names, else none. */
export function pathOfCall(tool: ToolName | undefined, summary: string, display: ToolDisplay | undefined): string {
  if (display?.kind === 'diff') {
    return display.path;
  }
  return tool !== undefined && FILE_TOOLS.includes(tool) ? summary : '';
}
