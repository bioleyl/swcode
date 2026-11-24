/**
 * The entry a host ships (the VS Code extension's webview, `vscode.html`): swcode's UI without the app's page
 * furniture. The host gives the daemon's address and its token in the query (`?daemon=http://127.0.0.1:4500&token=…`),
 * and its theme (`?theme=dark`); its colours arrive as `--jl-*` custom properties on the root (see `vscode.css`).
 */
import '@fontsource-variable/manrope';
import '@fontsource-variable/jetbrains-mono';
import './vscode.css';

import { defineRoutes } from '@jadis/core';

import './lib/icons';
import { HomePage } from './pages/home-page';
import { SessionPage } from './pages/session-page';
import { SessionsPage } from './pages/sessions-page';
import { daemon } from './services/daemon';
import { watchStatus } from './services/daemon-status';
import { hostConfig } from './services/host';
import { watchHostMessages } from './services/host-messages';
import './services/presets';
import './services/session-list';
import { applyTheme } from './services/theme';
import { ScAppShell } from './templates/sc-app-shell';

// The screens of the host's bundle: a session, every session, and the form a new one starts from. The app's other
// pages (the project, the settings, the workflows, the games) stay out of it.
// biome-ignore assist/source/useSortedKeys: the order of the routes is the order they are matched in
const HOST_ROUTES = defineRoutes({
  session: { page: SessionPage, path: '/sessions/:id' },
  sessions: { page: SessionsPage, path: '/sessions' },
  newSession: { page: HomePage, path: '/new' },
  home: { page: HomePage, path: '/' },
  notFound: { page: HomePage, path: '/:rest' },
});

// The page lives as long as the webview: its bus listeners never need to stop.
const forever = new AbortController().signal;

applyTheme(hostConfig().theme);
watchHostMessages();
const shell = new ScAppShell();
shell.compact = true;
shell.routes = HOST_ROUTES;
document.body.replaceChildren(shell);
daemon.connect();
watchStatus(forever);
