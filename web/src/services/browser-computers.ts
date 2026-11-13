import { Bus } from '@jadis/core';

import { THIS_COMPUTER } from '../../../shared/protocol/desktop.schema';
import { BEARER_PREFIX, ENDPOINTS } from '../../../shared/protocol/endpoints';
import { StatusSchema } from '../../../shared/protocol/messages.schema';
import { PairAnswerSchema } from '../../../shared/protocol/pairing.schema';
import { pairingTarget } from '../lib/remote-computers';
import { homeToken } from './access';
import { keepComputers, storedComputers } from './computer-store';
import { deviceName } from './pairing';

import type { Computer, DesktopAnswer } from '../../../shared/protocol/desktop.schema';
import type { ComputerRequest, PairingTarget, StoredComputer } from '../types/computers.types';

// How long a computer has to answer before it counts as out of reach.
const REACH_TIMEOUT_MS = 2000;
const LOOPBACK_NAMES: ReadonlySet<string> = new Set(['localhost', '127.0.0.1', '[::1]']);
const THIS_COMPUTER_NAME = 'This computer';
const SAME_COMPUTER = 'That is the computer this page came from: it is in the list already.';
const NO_ANSWER = 'Nothing answered there as swcode. Is it running, and reachable from this device?';
const CODE_REFUSED =
  'That computer refused the code: it works once and changes every minute. Read a new one in its Settings → Computers.';
const JSON_TYPE = { 'content-type': 'application/json' };

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

/**
 * The computers, at once as they are kept, then again once each one answered or did not: a page shows its list
 * without waiting for a computer that is off.
 */
async function list(): Promise<void> {
  const kept = storedComputers();
  const listed = (reached: readonly boolean[]): void =>
    answer({
      computers: [
        homeComputer(reached[0] ?? true),
        ...kept.computers.map((computer, index) => shownAs(computer, reached[index + 1] ?? true)),
      ],
      current: kept.current,
      type: 'computers',
    });
  listed([]);
  home.name ||= await machineName(location.origin, homeToken());
  listed(await Promise.all([location.origin, ...kept.computers.map((computer) => computer.address)].map(reaches)));
}

/** The page loads again at its first address, talking to the computer now kept as shown: another one's sessions and
 * projects are not this one's. */
function showAnother(id: string): void {
  keepComputers({ ...storedComputers(), current: id });
  history.replaceState(null, '', location.pathname);
  location.reload();
}

/** Trades a pairing code for a token of this browser's own, at another computer's swcode; why not, otherwise. */
async function pair(target: PairingTarget): Promise<string | undefined> {
  const response = await fetch(`${target.origin}${ENDPOINTS.pairing}`, {
    body: JSON.stringify({ code: target.code, name: deviceName(navigator.userAgent) }),
    headers: JSON_TYPE,
    method: 'POST',
  }).catch(() => undefined);
  if (!response) {
    refuse(NO_ANSWER);
    return undefined;
  }
  const paired = PairAnswerSchema.safeParse(await response.json().catch(() => undefined));
  if (!response.ok || !paired.success) {
    refuse(CODE_REFUSED);
    return undefined;
  }
  return paired.data.token;
}

async function add(address: string, code: string): Promise<void> {
  const target = pairingTarget(address, code, location.protocol);
  if (!target.ok) {
    refuse(target.problem);
    return;
  }
  if (target.answer.origin === location.origin) {
    refuse(SAME_COMPUTER);
    return;
  }
  const token = await pair(target.answer);
  if (token === undefined) {
    return;
  }
  const { origin } = target.answer;
  const kept = storedComputers();
  const added = { address: origin, id: crypto.randomUUID(), name: await machineName(origin, token), token };
  // Paired again, a computer takes its new token, and keeps the name and colour it was given here.
  const known = kept.computers.find((computer) => computer.address === origin);
  const others = kept.computers.filter((computer) => computer !== known);
  keepComputers({ ...kept, computers: [...others, known ? { ...known, token } : added] });
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
  addComputer: (request) => add(request.address, request.code ?? ''),
  editComputer: (request) => edit(request),
  forgetComputer: (request) => forget(request.id),
  listComputers: () => list(),
  openComputer: (request) => open(request.id),
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
