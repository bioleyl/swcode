import '@fontsource-variable/manrope';
import '@fontsource-variable/jetbrains-mono';
import './styles/global.css';
import './services/presets';
import './services/session-list';

import './lib/icons';

import { adoptToken } from './services/access';
import { daemon, daemonBus } from './services/daemon';
import { refreshStatus, watchStatus } from './services/daemon-status';
import { startApp } from './services/install';
import { pairFromLink } from './services/pairing';
import { followPushes } from './services/push';
import { applyTheme } from './services/theme';
import { ScAppShell } from './templates/sc-app-shell';

// The page lives as long as the tab: its bus listeners never need to stop.
const forever = new AbortController().signal;

applyTheme();
adoptToken();
document.body.replaceChildren(new ScAppShell());
startApp(forever);
followPushes(forever);
// The daemon back after a restart: ask at once whether llm-server answers too.
daemonBus.register('online', () => void refreshStatus(), forever);
// A phone that wakes, or a network that comes back: the socket is tried at once, or asked whether it still lives.
document.addEventListener(
  'visibilitychange',
  () => (document.visibilityState === 'visible' ? daemon.wake() : undefined),
  {
    signal: forever,
  }
);
window.addEventListener('online', () => daemon.wake(), { signal: forever });
// A phone opened from the QR code trades its pairing code for a token before it talks to the daemon.
void pairFromLink().finally(() => {
  daemon.connect();
  watchStatus(forever);
});
