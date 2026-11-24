import { DAEMON_PARAM, HOST_PAGE, OPEN_MESSAGE, THEME_PARAM, TOKEN_PARAM } from '../swcode.constants';

import type { DaemonTarget } from '../types/daemon.types';
import type { HostTheme } from '../types/webview-html.types';

const FRAME_STYLE =
  'html, body { height: 100%; margin: 0; } ' +
  '.swcode-frame { display: block; width: 100%; height: 100%; border: 0; }';

// The extension posts its messages to the webview's window and the bundle lives in the frame, so they are handed
// over both ways: the bundle's open request goes back to the extension, everything else comes into the frame.
const FORWARD_SCRIPT =
  'const vscode = typeof acquireVsCodeApi === "function" ? acquireVsCodeApi() : undefined; ' +
  "window.addEventListener('message', (event) => { " +
  'const data = event.data; ' +
  `if (data && data.type === '${OPEN_MESSAGE}') { if (vscode) { vscode.postMessage(data); } return; } ` +
  "const frame = document.querySelector('.swcode-frame'); " +
  "if (frame && frame.contentWindow) { frame.contentWindow.postMessage(data, '*'); } });";

/** The origin of the daemon's address, which the webview's page may load. */
function originOf(daemon: string): string {
  return new URL(daemon).origin;
}

/**
 * The bundle's page on the daemon, with what the host tells it: the daemon to talk to, the access token, and the
 * host's theme (`web/vscode.html`, the shared bundle's entry).
 */
export function pageUrl(target: DaemonTarget, theme: HostTheme): string {
  const params = new URLSearchParams({
    [DAEMON_PARAM]: target.daemon,
    [TOKEN_PARAM]: target.token,
    [THEME_PARAM]: theme,
  });
  return `${target.daemon}/${HOST_PAGE}?${params.toString()}`;
}

/**
 * The webview's own page: the bundle's page in a frame that fills the panel. The frame inherits the IDE's
 * `--vscode-*` colours, which `web/src/vscode.css` maps onto jlib's `--jl-*` tokens.
 */
export function frameHtml(target: DaemonTarget, theme: HostTheme): string {
  const origin = originOf(target.daemon);
  const policy = `default-src 'none'; frame-src ${origin}; style-src 'unsafe-inline';`;
  return [
    '<!doctype html>',
    '<html lang="en">',
    '<head>',
    '<meta charset="utf-8" />',
    '<meta name="viewport" content="width=device-width, initial-scale=1.0" />',
    `<meta http-equiv="Content-Security-Policy" content="${policy}" />`,
    `<style>${FRAME_STYLE}</style>`,
    '</head>',
    '<body>',
    `<iframe class="swcode-frame" src="${pageUrl(target, theme)}" title="swcode"></iframe>`,
    `<script>${FORWARD_SCRIPT}</script>`,
    '</body>',
    '</html>',
  ].join('\n');
}
