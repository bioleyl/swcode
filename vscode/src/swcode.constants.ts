// Values several modules of the extension share: the ids VS Code is given, and where the daemon is found.

/** The view container in the activity bar, and the webview view inside it (both in package.json's `contributes`). */
export const VIEW_CONTAINER_ID = 'swcode';
export const VIEW_ID = 'swcode.sessionPanel';

/** The command that asks swcode about the editor's selection, from the menu and the palette. */
export const ASK_COMMAND = 'swcode.ask';
export const ASK_TITLE = 'Ask swcode';
/** What "Ask swcode" asks: nothing of the person's own, the composer puts the editor's context in front of it. */
export const ASK_PROMPT = '';

/**
 * The commands that act on the code with a preset prompt, from the palette and the editor's context menu.
 * "Fix problems" is the one that asks for the diagnostics along with the selection; the others leave the
 * composer's "Include problems" toggle to the person.
 */
export const REFACTOR_COMMAND = 'swcode.refactor';
export const REFACTOR_TITLE = 'Refactor';
export const REFACTOR_PROMPT = 'Refactor the selected code, keeping its behaviour.';
export const EXPLAIN_COMMAND = 'swcode.explain';
export const EXPLAIN_TITLE = 'Explain';
export const EXPLAIN_PROMPT = 'Explain what this code does.';
export const FIX_PROBLEMS_COMMAND = 'swcode.fixProblems';
export const FIX_PROBLEMS_TITLE = 'Fix problems';
export const FIX_PROBLEMS_PROMPT = 'Fix the problems reported for this file.';
export const ADD_TESTS_COMMAND = 'swcode.addTests';
export const ADD_TESTS_TITLE = 'Add tests';
export const ADD_TESTS_PROMPT = 'Add tests for the selected code.';

/** The bundle a host ships (web/vscode.html, built to dist/web). */
export const HOST_PAGE = 'vscode.html';

/** The daemon's address and token, as the shared bundle reads them from the page's query. */
export const DAEMON_PARAM = 'daemon';
export const THEME_PARAM = 'theme';
export const TOKEN_PARAM = 'token';

/** The data folder's files: the settings the settings page writes, and the access token the daemon created. */
export const SETTINGS_FILE = 'settings.env';
export const TOKEN_FILE = 'token';
export const DATA_DIR_ENV = 'SWCODE_DATA_DIR';
export const DATA_DIR_NAME = '.swcode';

export const PORT_ENV = 'SWCODE_PORT';
export const DEFAULT_PORT = 4500;
export const MAX_PORT = 65_535;

/** The messages the extension posts into the webview, and the bundle reads back. */
export const CONTEXT_MESSAGE = 'context';
export const PROMPT_MESSAGE = 'prompt';
/** The message the bundle posts back, asking the editor to open a file. */
export const OPEN_MESSAGE = 'open';

/** What the status bar says when a file cannot be opened, and how long it says it for. */
export const OPEN_MESSAGE_FAILURE = 'swcode: the file could not be opened';
export const OPEN_MESSAGE_TIMEOUT_MS = 4_000;

/** How long the editor's selection and the problems settle before the panel is told about them. */
export const CONTEXT_REFRESH_MS = 150;

/** The status bar asks the daemon for its socket's status, as the app's page does. */
export const STATUS_POLL_MS = 15_000;
export const PROBE_TIMEOUT_MS = 1_000;
export const STATUS_BAR_PRIORITY = 100;

/** The status bar's text, with the daemon's state. */
export const STATUS_CONNECTED = 'swcode: connected';
export const STATUS_NOT_RUNNING = 'swcode: daemon not running';
