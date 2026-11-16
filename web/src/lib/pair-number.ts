import { PAIR_NUMBER_DIGITS } from '../../../shared/protocol/pairing.schema';

const PADDING = '0';
const NUMBERS = 10 ** PAIR_NUMBER_DIGITS;
const UINT32 = 0x1_0000_0000;

/**
 * The number an app shows to pair: six digits a person types on the computer, which makes them wait with
 * `swcode pair`. Drawn from the browser's own random numbers, so that a number cannot be guessed from another.
 */
export function newPairNumber(
  random: () => number = () => (crypto.getRandomValues(new Uint32Array(1))[0] ?? 0) / UINT32
): string {
  return String(Math.floor(random() * NUMBERS)).padStart(PAIR_NUMBER_DIGITS, PADDING);
}
