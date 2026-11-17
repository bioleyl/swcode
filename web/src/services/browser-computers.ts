import { Bus } from '@jadis/core';

import { THIS_COMPUTER } from '../../../shared/protocol/desktop.schema';
import { BEARER_PREFIX, ENDPOINTS } from '../../../shared/protocol/endpoints';
import { StatusSchema } from '../../../shared/protocol/messages.schema';
import { homeToken } from './access';
import { keepComputers, storedComputers } from './computer-store';
import { inAndroidApp } from './native-app';

import type { Computer, DesktopAnswer } from '../../../shared/protocol/desktop.schema';
import type { ComputerRequest, StoredComputer, StoredComputers } from '../types/computers.types';

// How long a computer has to answer before it counts as out of reach.
const REACH_TIMEOUT_MS = 2000;
const LOOPBACK_NAMES: ReadonlySet<string> = new Set(['localhost', '127.0.0.1', '[::1]']);
const THIS_COMPUTER_NAME = 'This computer';
const SAME_COMPUTER = 'That is the computer this page came from: it is in the list already.';

/**
 * What the browser answers about the computers it shows, the way the desktop app answers in its window: the pages
 * hear both the same way.
 */
export const browserComputersBus = new Bus<{ answer: DesktopAnswer }>();

const answer = (said: DesktopAnswer): void => browserComputersBus.emit('answer', said);

const refuse = (message: string): void => answer({ message, type: 'computerRefused' });

/** Whether a swcode answers at an address, soon enough to be shown. */
const reaches = (origin: string): Promise<boolean> =>
  fetch(`${origin}${ENDPOINTS.health}`, { signal: AbortSignal.timeout(REACH_TIMEOUT_MS) })
    .then((response) => response.ok)
    .catch(() => false);

/** The name of the machine a swcode runs on, as its status says; the address's own name when it does not say. */
async function machineName(origin: string, token: string | undefined): Promise<string> {
  const response = await fetch(`${origin}${ENDPOINTS.status}`, {
    headers: token ? { authorization: `${BEARER_PREFIX}${token}` } : {},
    signal: AbortSignal.timeout(REACH_TIMEOUT_MS),
  }).catch(() => undefined);
  const status = StatusSchema.safeParse(await response?.json().catch(() => undefined));
  return (status.success ? status.data.computer : undefined) ?? new URL(origin).hostname;
}

// The name of the machine that served the page, once its status said it: asked once a page.
const home = { name: LOOPBACK_NAMES.has(location.hostname) ? THIS_COMPUTER_NAME : '' };

/** The computer that served the page: this one for a page opened on it, named after its machine anywhere else. */
const homeComputer = (reachable: boolean): Computer => ({
  address: location.origin,
  id: THIS_COMPUTER,
  name: home.name || location.hostname,
  reachable,
});

const shownAs = (kept: StoredComputer, reachable: boolean): Computer => ({
  address: kept.address,
  id: kept.id,
  name: kept.name,
  reachable,
  ...(kept.color === undefined ? {} : { color: kept.color }),
});

/** Whether the page shows the computer that served it: swcode's Android app has no swcode of its own. */
const showsHome = (): boolean => !inAndroidApp();

/** The addresses the list checks: the page's own computer first, where there is one. */
const addressesOf = (kept: StoredComputers): string[] => {
  const others = kept.computers.map((computer) => computer.address);
  return showsHome() ? [location.origin, ...others] : others;
};

/** The computers the list holds: the one that served the page, then those kept; on a phone, only the others. */
const computersOf = (kept: StoredComputers, reached: readonly boolean[]): Computer[] => {
  const first = showsHome() ? 1 : 0;
  const others = kept.computers.map((computer, index) => shownAs(computer, reached[index + first] ?? true));
  return showsHome() ? [homeComputer(reached[0] ?? true), ...others] : others;
};

/** The computer shown: on a phone, one of the others even when it was swcode's own that was kept. */
const currentOf = (kept: StoredComputers): string => {
  if (showsHome()) {
    return kept.current;
  }
  const shown = kept.computers.some((computer) => computer.id === kept.current) ? kept.current : '';
  return shown || (kept.computers[0]?.id ?? '');
};

/**
 * The computers, at once as they are kept, then again once each one answered or did not: a page shows its list
 * without waiting for a computer that is off.
 */
async function list(): Promise<void> {
  const kept = storedComputers();
  const listed = (reached: readonly boolean[]): void =>
    answer({ computers: computersOf(kept, reached), current: currentOf(kept), type: 'computers' });
  listed([]);
  if (showsHome()) {
    home.name ||= await machineName(location.origin, homeToken());
  }
  listed(await Promise.all(addressesOf(kept).map(reaches)));
}

/** The page loads again where the computer now shown answers: another one's sessions and projects are not this one's. */
function showAnother(id: string): void {
  keepComputers({ ...storedComputers(), current: id });
  history.replaceState(null, '', location.pathname);
  location.reload();
}

/** Keeps a computer its pairing gave this browser a token for, in place of one at the same address, and its id. */
function keep(address: string, name: string, token: string): StoredComputer {
  const kept = storedComputers();
  // Paired again, a computer takes its new token, and keeps the name and colour it was given here.
  const known = kept.computers.find((computer) => computer.address === address);
  const computer = known ? { ...known, token } : { address, id: crypto.randomUUID(), name, token };
  keepComputers({ ...kept, computers: [...kept.computers.filter((each) => each !== known), computer] });
  return computer;
}

/**
 * Keeps the computer a page paired with and shows it at once: the very first one, from a page no daemon serves, which
 * has nothing else to show.
 */
export function connectTo(address: string, name: string, token: string): void {
  showAnother(keep(address, name, token).id);
}

async function add(request: Extract<ComputerRequest, { type: 'addComputer' }>): Promise<void> {
  if (request.address === location.origin) {
    refuse(SAME_COMPUTER);
    return;
  }
  keep(request.address, request.name, request.token);
  await list();
}

async function open(id: string): Promise<void> {
  const computer = storedComputers().computers.find((each) => each.id === id);
  if (computer && !(await reaches(computer.address))) {
    answer({ id, name: computer.name, type: 'computerUnreachable' });
    return;
  }
  showAnother(computer ? id : THIS_COMPUTER);
}

async function edit(request: Extract<ComputerRequest, { type: 'editComputer' }>): Promise<void> {
  const kept = storedComputers();
  const computers = kept.computers.map((computer) =>
    computer.id === request.id ? { ...computer, color: request.color, name: request.name } : computer
  );
  keepComputers({ ...kept, computers });
  await list();
}

async function forget(id: string): Promise<void> {
  const kept = storedComputers();
  keepComputers({ ...kept, computers: kept.computers.filter((computer) => computer.id !== id) });
  // Its sessions are on show: the page goes back to the computer it came from.
  if (kept.current === id) {
    showAnother(THIS_COMPUTER);
    return;
  }
  await list();
}

const ANSWERS: {
  [Type in ComputerRequest['type']]: (request: Extract<ComputerRequest, { type: Type }>) => Promise<void>;
} = {
  addComputer: (request) => add(request),
  editComputer: (request) => edit(request),
  forgetComputer: (request) => forget(request.id),
  listComputers: () => list(),
  // A browser keeps where its notifications stand in its own push subscription, which the page reads itself.
  notifications: () => list(),
  openComputer: (request) => open(request.id),
  setNotifications: () => list(),
};

const answerTo = <Type extends ComputerRequest['type']>(
  type: Type,
  request: Extract<ComputerRequest, { type: Type }>
): Promise<void> => ANSWERS[type](request);

/**
 * Answers a page about the computers this browser shows: the one that served the page, and the others it paired
 * with, each reached at its own address with the token its pairing gave. The answers come on the bus.
 */
export function askBrowser(request: ComputerRequest): void {
  void answerTo(request.type, request);
}
