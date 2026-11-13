import { PAIR_PARAM, TOKEN_PARAM } from '../../../shared/protocol/endpoints';

import type { Answer } from '../types/answer.types';
import type { PairingTarget } from '../types/computers.types';

const SCHEME_MARK = '://';
const SECURE = 'https:';
const PLAIN = 'http:';
// The port swcode listens on when it was given none: a plain address without one means that port.
const DEFAULT_PORT = '4500';

export const NO_ADDRESS = 'Type that computer’s address, or paste the link under its QR code.';
export const NO_CODE =
  'Type the pairing code that computer shows in Settings → Computers, or paste the link under its QR code, which holds one.';
export const TOKEN_LINK =
  'A link with a token opens that computer’s own swcode. To show it here, use its pairing code or the link under its QR code.';
export const NOT_SECURE =
  'This page came over HTTPS, and a browser lets it talk only to addresses that are HTTPS too. Give that computer one (a tailnet does: tailscale serve), then add it by that address.';

const urlOf = (text: string): URL | undefined => {
  try {
    return new URL(text);
  } catch {
    return undefined;
  }
};

/**
 * Where a computer to add is and the code that pairs with it, from what the person typed or scanned: the link under
 * a QR code holds both, an address needs the code typed beside it. An address without a scheme takes the page's
 * own, and a plain one without a port swcode's usual one. Refused: no address, no code, a link that carries a token
 * (which a page of another computer must never hold), and a plain address from a page that came over HTTPS.
 */
export function pairingTarget(typed: string, typedCode: string, pageProtocol: string): Answer<PairingTarget> {
  const text = typed.trim();
  const link = urlOf(text.includes(SCHEME_MARK) ? text : `${pageProtocol}//${text}`);
  if (text === '' || !link || link.hostname === '') {
    return { ok: false, problem: NO_ADDRESS };
  }
  if (link.searchParams.has(TOKEN_PARAM)) {
    return { ok: false, problem: TOKEN_LINK };
  }
  const code = link.searchParams.get(PAIR_PARAM) ?? typedCode.trim();
  if (code === '') {
    return { ok: false, problem: NO_CODE };
  }
  if (pageProtocol === SECURE && link.protocol !== SECURE) {
    return { ok: false, problem: NOT_SECURE };
  }
  if (link.protocol === PLAIN && link.port === '') {
    link.port = DEFAULT_PORT;
  }
  return { answer: { code, origin: link.origin }, ok: true };
}
