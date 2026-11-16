import type { Answer } from '../types/answer.types';

const SCHEME_MARK = '://';
const SECURE = 'https:';
const PLAIN = 'http:';
// The port swcode listens on when it was given none: 4500, and the address form's own.
export const DEFAULT_PORT = '4500';
const LOWEST_PORT = 1;
const HIGHEST_PORT = 65_535;

export const NO_ADDRESS = 'Type that computer’s address: its name or IP address.';
export const BAD_PORT = 'The port is a number from 1 to 65535, or nothing for 4500.';
export const NOT_SECURE =
  'This page came over HTTPS, and a browser lets it talk only to addresses that are HTTPS too. Give that computer one (a tailnet does: tailscale serve), then add it by that address.';

const urlOf = (text: string): URL | undefined => {
  try {
    return new URL(text);
  } catch {
    return undefined;
  }
};

const portOf = (typed: string): string | undefined => {
  const number = Number(typed);
  if (!Number.isInteger(number) || number < LOWEST_PORT || number > HIGHEST_PORT) {
    return undefined;
  }
  return String(number);
};

// The port an address without one takes: swcode's 4500 over plain HTTP; over HTTPS the link keeps its own, which is
// the tailnet's 443 when it says nothing.
const defaultPortOf = (link: URL): string | undefined => {
  if (link.port !== '') {
    return link.port;
  }
  return link.protocol === SECURE ? undefined : DEFAULT_PORT;
};

/**
 * Where the computer to add answers, from the address and the port the person typed: a name or an IP address, or a
 * whole link. An address without a scheme takes the page's own; a port left out is swcode's 4500 over plain HTTP, and
 * the link's own over HTTPS. Refused: no address, a port that is not one, and a plain address from a page that came
 * over HTTPS, which no browser would let the page reach.
 */
export function pairingOrigin(typed: string, typedPort: string, pageProtocol: string): Answer<string> {
  const text = typed.trim();
  const link = urlOf(text.includes(SCHEME_MARK) ? text : `${pageProtocol}//${text}`);
  if (text === '' || !link || link.hostname === '') {
    return { ok: false, problem: NO_ADDRESS };
  }
  if (link.protocol !== PLAIN && link.protocol !== SECURE) {
    return { ok: false, problem: NO_ADDRESS };
  }
  const port = typedPort.trim() === '' ? defaultPortOf(link) : portOf(typedPort.trim());
  if (typedPort.trim() !== '' && port === undefined) {
    return { ok: false, problem: BAD_PORT };
  }
  if (pageProtocol === SECURE && link.protocol !== SECURE) {
    return { ok: false, problem: NOT_SECURE };
  }
  if (port !== undefined) {
    link.port = port;
  }
  return { answer: link.origin, ok: true };
}
