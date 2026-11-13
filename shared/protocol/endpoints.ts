/** The daemon's addresses, shared by the server that answers them and the app that calls them. */
export const API_PREFIX = '/api/';

export const ENDPOINTS = {
  // GET: what closing the desktop app would stop, and the warning its window shows first.
  closing: '/api/closing',
  // GET: the paired phones; DELETE /api/devices/<id> forgets one, its token with it.
  devices: '/api/devices',
  health: '/api/health',
  // GET /api/images/<session>/<file>: a picture the agent made, as the daemon keeps it to show.
  images: '/api/images',
  // GET the MCP servers the daemon reaches and where each stands; POST reads their file again.
  mcp: '/api/mcp',
  // GET the llm-servers and the presets, with their keys; PUT them all, in use at once.
  models: '/api/models',
  // POST a pairing code, without a token: the phone gets its own. Open to anyone, the code being the secret.
  pairing: '/api/pairing',
  // GET a new pairing code for the QR code, valid a minute and once.
  pairingCode: '/api/pairing/code',
  // GET the settings in use, PUT new ones (the daemon restarts).
  settings: '/api/settings',
  // POST an llm-server's address: the models it serves.
  settingsModels: '/api/settings/models',
  // GET ?project=<id>: the skills the project's sessions find, read on its host.
  skills: '/api/skills',
  socket: '/api/socket',
  // GET whether this daemon starts with its computer, and how; PUT how it should (the Windows app only).
  startup: '/api/startup',
  status: '/api/status',
  // POST: the daemon shuts down, as on Ctrl+C.
  stop: '/api/stop',
  // GET where the swcode-base skill is, with ?project=<id>; POST has npx skills put it in the project, DELETE take
  // it out of it. All on the project's host.
  swcodeBase: '/api/swcode-base',
  // PUT has npx skills install swcode-base in the home, DELETE take it out of it: the home on the host of the
  // ?project=<id>, this computer's without one.
  swcodeBaseHome: '/api/swcode-base/home',
  // GET whether this computer's tailnet can give swcode an HTTPS address, and the address; POST has Tailscale serve
  // swcode there.
  tailnet: '/api/tailnet',
  // GET the version that runs and what is known of a newer one; PUT whether to look every 10 minutes.
  update: '/api/update',
  // POST: look for a newer version now.
  updateCheck: '/api/update/check',
  // POST: install the newer version found, in place of the program that runs, which then restarts.
  updateInstall: '/api/update/install',
} as const;

/** The access token travels in this query parameter: a browser cannot set headers on a WebSocket. */
export const TOKEN_PARAM = 'token';

/** The project whose skills `GET /api/skills` lists travels in this query parameter. */
export const PROJECT_PARAM = 'project';

/** The pairing code travels in this query parameter of the QR code's link. */
export const PAIR_PARAM = 'pair';

export const BEARER_PREFIX = 'Bearer ';
