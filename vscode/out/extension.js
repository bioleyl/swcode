"use strict";
var __defProp = Object.defineProperty;
var __getOwnPropDesc = Object.getOwnPropertyDescriptor;
var __getOwnPropNames = Object.getOwnPropertyNames;
var __hasOwnProp = Object.prototype.hasOwnProperty;
var __export = (target, all) => {
  for (var name in all)
    __defProp(target, name, { get: all[name], enumerable: true });
};
var __copyProps = (to, from, except, desc) => {
  if (from && typeof from === "object" || typeof from === "function") {
    for (let key of __getOwnPropNames(from))
      if (!__hasOwnProp.call(to, key) && key !== except)
        __defProp(to, key, { get: () => from[key], enumerable: !(desc = __getOwnPropDesc(from, key)) || desc.enumerable });
  }
  return to;
};
var __toCommonJS = (mod) => __copyProps(__defProp({}, "__esModule", { value: true }), mod);

// src/extension.ts
var extension_exports = {};
__export(extension_exports, {
  activate: () => activate
});
module.exports = __toCommonJS(extension_exports);
var import_vscode6 = require("vscode");

// src/daemon.ts
var import_node_fs = require("node:fs");
var import_node_os = require("node:os");
var import_node_path = require("node:path");
var import_node_net = require("node:net");

// src/swcode.constants.ts
var VIEW_CONTAINER_ID = "swcode";
var VIEW_ID = "swcode.sessionPanel";
var ASK_COMMAND = "swcode.ask";
var ASK_PROMPT = "";
var REFACTOR_COMMAND = "swcode.refactor";
var REFACTOR_PROMPT = "Refactor the selected code, keeping its behaviour.";
var EXPLAIN_COMMAND = "swcode.explain";
var EXPLAIN_PROMPT = "Explain what this code does.";
var FIX_PROBLEMS_COMMAND = "swcode.fixProblems";
var FIX_PROBLEMS_PROMPT = "Fix the problems reported for this file.";
var ADD_TESTS_COMMAND = "swcode.addTests";
var ADD_TESTS_PROMPT = "Add tests for the selected code.";
var HOST_PAGE = "vscode.html";
var DAEMON_PARAM = "daemon";
var THEME_PARAM = "theme";
var TOKEN_PARAM = "token";
var SETTINGS_FILE = "settings.env";
var TOKEN_FILE = "token";
var DATA_DIR_ENV = "SWCODE_DATA_DIR";
var DATA_DIR_NAME = ".swcode";
var PORT_ENV = "SWCODE_PORT";
var DEFAULT_PORT = 4500;
var MAX_PORT = 65535;
var CONTEXT_MESSAGE = "context";
var PROMPT_MESSAGE = "prompt";
var OPEN_MESSAGE = "open";
var OPEN_MESSAGE_FAILURE = "swcode: the file could not be opened";
var OPEN_MESSAGE_TIMEOUT_MS = 4e3;
var CONTEXT_REFRESH_MS = 150;
var STATUS_POLL_MS = 15e3;
var PROBE_TIMEOUT_MS = 1e3;
var STATUS_BAR_PRIORITY = 100;
var STATUS_CONNECTED = "swcode: connected";
var STATUS_NOT_RUNNING = "swcode: daemon not running";

// src/lib/settings-env.ts
var COMMENT_MARK = "#";
var EXPORT_PREFIX = "export ";
var QUOTES = ["'", '"', "`"];
var ESCAPED_NEWLINE = "\\n";
var NEWLINE = "\n";
var PORT_DIGITS = /^\d+$/;
var EQUALS_MARK = "=";
function unquoted(value) {
  const opening = value[0] ?? "";
  if (!QUOTES.includes(opening) || value.at(-1) !== opening || value.length < 2) {
    return value;
  }
  return value.slice(1, -1).replaceAll(ESCAPED_NEWLINE, NEWLINE);
}
function settingOf(line) {
  const trimmed = line.trim();
  if (trimmed === "" || trimmed.startsWith(COMMENT_MARK)) {
    return void 0;
  }
  const assignment = trimmed.indexOf(EQUALS_MARK);
  if (assignment < 0) {
    return void 0;
  }
  const key = trimmed.slice(trimmed.startsWith(EXPORT_PREFIX) ? EXPORT_PREFIX.length : 0, assignment).trim();
  if (key === "") {
    return void 0;
  }
  return { key, value: unquoted(trimmed.slice(assignment + EQUALS_MARK.length)) };
}
function parseSettingsEnv(text) {
  const settings = {};
  for (const line of text.split(NEWLINE)) {
    const setting = settingOf(line);
    if (setting) {
      settings[setting.key] = setting.value;
    }
  }
  return settings;
}
function portOf(settings) {
  const saved = settings[PORT_ENV] ?? "";
  if (!PORT_DIGITS.test(saved)) {
    return DEFAULT_PORT;
  }
  const port = Number(saved);
  return port > 0 && port <= MAX_PORT ? port : DEFAULT_PORT;
}

// src/daemon.ts
var LOCAL_HOST = "127.0.0.1";
var SCHEME = "http://";
var PORT_SEPARATOR = ":";
var UTF8 = "utf8";
var FALLBACK_TEXT = "";
function dataDirOf(environment) {
  const given = environment[DATA_DIR_ENV];
  return given && given !== "" ? given : (0, import_node_path.join)((0, import_node_os.homedir)(), DATA_DIR_NAME);
}
function readText(path) {
  try {
    return (0, import_node_fs.readFileSync)(path, UTF8);
  } catch {
    return FALLBACK_TEXT;
  }
}
function readTarget(environment = process.env) {
  const dir = dataDirOf(environment);
  const port = portOf(parseSettingsEnv(readText((0, import_node_path.join)(dir, SETTINGS_FILE))));
  return { daemon: `${SCHEME}${LOCAL_HOST}:${port}`, token: readText((0, import_node_path.join)(dir, TOKEN_FILE)).trim() };
}
function addressOf(target) {
  return target.daemon.slice(SCHEME.length);
}
function portOfTarget(target) {
  const separator = addressOf(target).indexOf(PORT_SEPARATOR);
  return separator < 0 ? DEFAULT_PORT : Number(addressOf(target).slice(separator + 1));
}
function probe(target, timeoutMs = PROBE_TIMEOUT_MS) {
  const socket = new import_node_net.Socket();
  socket.setTimeout(timeoutMs);
  return new Promise((resolve) => {
    const settle = (probeResult) => {
      socket.destroy();
      resolve(probeResult);
    };
    socket.connect(portOfTarget(target), LOCAL_HOST);
    socket.once("connect", () => settle({ kind: "connected" }));
    socket.once("timeout", () => settle({ kind: "unreachable", problem: "timed out" }));
    socket.once("error", (error) => settle({ kind: "unreachable", problem: error.message }));
  });
}

// src/session-panel.ts
var import_vscode4 = require("vscode");

// src/context.ts
var import_vscode = require("vscode");
var SEVERITY_NAMES = { 0: "error", 1: "warning", 2: "info", 3: "hint" };
var severityOf = (severity) => SEVERITY_NAMES[severity] ?? "info";
var pathOf = (editor) => editor.document.uri.fsPath;
var selectionOf = (editor) => ({
  endColumn: editor.selection.end.character + 1,
  endLine: editor.selection.end.line + 1,
  startColumn: editor.selection.start.character + 1,
  startLine: editor.selection.start.line + 1,
  text: editor.document.getText(editor.selection)
});
var diagnosticsOf = (editor) => import_vscode.languages.getDiagnostics(editor.document.uri).map((diagnostic) => ({
  code: diagnostic.code?.toString() ?? "",
  column: diagnostic.range.start.character + 1,
  line: diagnostic.range.start.line + 1,
  message: diagnostic.message,
  severity: severityOf(diagnostic.severity),
  source: diagnostic.source ?? ""
}));
function currentContext() {
  const editor = import_vscode.window.activeTextEditor;
  if (editor === void 0) {
    return void 0;
  }
  return {
    diagnostics: diagnosticsOf(editor),
    language: editor.document.languageId,
    path: pathOf(editor),
    selection: selectionOf(editor)
  };
}
function post(webview, message) {
  void webview.postMessage(message);
}
function startContext(view) {
  const push = () => {
    const context = currentContext();
    if (context !== void 0) {
      post(view.webview, { context, type: CONTEXT_MESSAGE });
    }
  };
  let refresh;
  const debounced = () => {
    clearTimeout(refresh);
    refresh = setTimeout(push, CONTEXT_REFRESH_MS);
  };
  const selectionChanges = import_vscode.window.onDidChangeTextEditorSelection((event) => {
    if (event.textEditor === import_vscode.window.activeTextEditor) {
      debounced();
    }
  });
  const diagnosticChanges = import_vscode.languages.onDidChangeDiagnostics(debounced);
  const visibility = view.onDidChangeVisibility(() => {
    if (view.visible) {
      push();
    }
  });
  push();
  return {
    dispose: () => {
      clearTimeout(refresh);
      selectionChanges.dispose();
      diagnosticChanges.dispose();
      visibility.dispose();
    }
  };
}
function ask(webview, text, includeProblems) {
  post(webview, { includeProblems, text, type: PROMPT_MESSAGE });
}

// src/open-file.ts
var import_vscode2 = require("vscode");
var isRecord = (value) => typeof value === "object" && value !== null;
var lineOf = (value) => typeof value === "number" ? value : void 0;
function openRequestFrom(data) {
  if (!isRecord(data) || data.type !== OPEN_MESSAGE) {
    return void 0;
  }
  const path = typeof data.path === "string" ? data.path : "";
  if (path === "") {
    return void 0;
  }
  return { endLine: lineOf(data.endLine), path, startLine: lineOf(data.startLine), type: OPEN_MESSAGE };
}
function rangeOf(request) {
  const startLine = request.startLine;
  if (startLine === void 0) {
    return void 0;
  }
  return new import_vscode2.Range(startLine - 1, 0, (request.endLine ?? startLine) - 1, 0);
}
function openInEditor(data) {
  const request = openRequestFrom(data);
  if (request === void 0) {
    return;
  }
  const uri = import_vscode2.Uri.file(request.path);
  import_vscode2.workspace.openTextDocument(uri).then(
    (document) => void import_vscode2.window.showTextDocument(document, { preview: true, selection: rangeOf(request) }),
    () => import_vscode2.window.setStatusBarMessage(OPEN_MESSAGE_FAILURE, OPEN_MESSAGE_TIMEOUT_MS)
  );
}

// src/lib/theme.ts
var import_vscode3 = require("vscode");
var THEMES = {
  [import_vscode3.ColorThemeKind.Light]: "light",
  [import_vscode3.ColorThemeKind.Dark]: "dark",
  [import_vscode3.ColorThemeKind.HighContrast]: "dark",
  [import_vscode3.ColorThemeKind.HighContrastLight]: "light"
};
function themeOf(kind) {
  return THEMES[kind];
}

// src/lib/webview-html.ts
var FRAME_STYLE = "html, body { height: 100%; margin: 0; } .swcode-frame { display: block; width: 100%; height: 100%; border: 0; }";
var FORWARD_SCRIPT = `const vscode = typeof acquireVsCodeApi === "function" ? acquireVsCodeApi() : undefined; window.addEventListener('message', (event) => { const data = event.data; if (data && data.type === '${OPEN_MESSAGE}') { if (vscode) { vscode.postMessage(data); } return; } const frame = document.querySelector('.swcode-frame'); if (frame && frame.contentWindow) { frame.contentWindow.postMessage(data, '*'); } });`;
function originOf(daemon) {
  return new URL(daemon).origin;
}
function pageUrl(target, theme) {
  const params = new URLSearchParams({
    [DAEMON_PARAM]: target.daemon,
    [TOKEN_PARAM]: target.token,
    [THEME_PARAM]: theme
  });
  return `${target.daemon}/${HOST_PAGE}?${params.toString()}`;
}
function frameHtml(target, theme) {
  const origin = originOf(target.daemon);
  const policy = `default-src 'none'; frame-src ${origin}; style-src 'unsafe-inline';`;
  return [
    "<!doctype html>",
    '<html lang="en">',
    "<head>",
    '<meta charset="utf-8" />',
    '<meta name="viewport" content="width=device-width, initial-scale=1.0" />',
    `<meta http-equiv="Content-Security-Policy" content="${policy}" />`,
    `<style>${FRAME_STYLE}</style>`,
    "</head>",
    "<body>",
    `<iframe class="swcode-frame" src="${pageUrl(target, theme)}" title="swcode"></iframe>`,
    `<script>${FORWARD_SCRIPT}</script>`,
    "</body>",
    "</html>"
  ].join("\n");
}

// src/session-panel.ts
var SessionPanel = class {
  constructor(target, subscriptions) {
    this.target = target;
    this.subscriptions = subscriptions;
  }
  target;
  subscriptions;
  #view;
  resolveWebviewView(view) {
    this.#view = view;
    view.webview.options = { enableScripts: true };
    this.#draw(view);
    this.subscriptions.push(startContext(view), view.webview.onDidReceiveMessage(openInEditor));
  }
  /** Puts the bundle's page back in the frame, with the theme the IDE wears now. */
  refresh() {
    if (this.#view) {
      this.#draw(this.#view);
    }
  }
  /** A command's prompt: the panel shows, and the prompt goes with the editor's context and the problems it asks for. */
  ask(text, includeProblems) {
    if (this.#view) {
      ask(this.#view.webview, text, includeProblems);
      this.#view.show(true);
    }
  }
  #draw(view) {
    view.webview.html = frameHtml(this.target, themeOf(import_vscode4.window.activeColorTheme.kind));
  }
};

// src/status-bar.ts
var import_vscode5 = require("vscode");
var STATE_TEXTS = {
  connected: STATUS_CONNECTED,
  unreachable: STATUS_NOT_RUNNING
};
var PROBE_FAILURE = "The daemon did not answer";
function createStatusBar(target) {
  const item = import_vscode5.window.createStatusBarItem(import_vscode5.StatusBarAlignment.Left, STATUS_BAR_PRIORITY);
  item.name = VIEW_CONTAINER_ID;
  const ask2 = () => void probe(target).then((state) => applyState(item, target, state));
  ask2();
  const timer = setInterval(ask2, STATUS_POLL_MS);
  return import_vscode5.Disposable.from(item, { dispose: () => clearInterval(timer) });
}
function applyState(item, target, state) {
  item.text = STATE_TEXTS[state.kind];
  const where = state.kind === "unreachable" ? PROBE_FAILURE : VIEW_CONTAINER_ID;
  item.tooltip = `${where} \u2014 ${addressOf(target)}`;
}

// src/extension.ts
function activate(context) {
  const target = readTarget();
  const panel = new SessionPanel(target, context.subscriptions);
  context.subscriptions.push(
    import_vscode6.window.registerWebviewViewProvider(VIEW_ID, panel),
    import_vscode6.window.onDidChangeActiveColorTheme(() => panel.refresh()),
    createStatusBar(target),
    import_vscode6.commands.registerCommand(ASK_COMMAND, () => panel.ask(ASK_PROMPT)),
    import_vscode6.commands.registerCommand(REFACTOR_COMMAND, () => panel.ask(REFACTOR_PROMPT)),
    import_vscode6.commands.registerCommand(EXPLAIN_COMMAND, () => panel.ask(EXPLAIN_PROMPT)),
    import_vscode6.commands.registerCommand(FIX_PROBLEMS_COMMAND, () => panel.ask(FIX_PROBLEMS_PROMPT, true)),
    import_vscode6.commands.registerCommand(ADD_TESTS_COMMAND, () => panel.ask(ADD_TESTS_PROMPT))
  );
}
// Annotate the CommonJS export names for ESM import in node:
0 && (module.exports = {
  activate
});
//# sourceMappingURL=extension.js.map
