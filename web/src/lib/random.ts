import type { Random } from '../types/game.types';

/** A number between `low` and `high`. */
export const between = (random: Random, low: number, high: number): number => low + random() * (high - low);

/** One of the keys, each as likely as its weight. */
export const pickWeighted = <Weights extends Readonly<Record<string, number>>>(
  weights: Weights,
  random: Random
): Extract<keyof Weights, string> => {
  const keys: Extract<keyof Weights, string>[] = [];
  for (const key in weights) {
    keys.push(key);
  }
  const weightUpTo = (index: number): number =>
    keys.slice(0, index + 1).reduce((sum, key) => sum + (weights[key] ?? 0), 0);
  const target = random() * weightUpTo(keys.length - 1);
  // The last one when the sums round below the target.
  const picked = keys.find((_, index) => weightUpTo(index) > target) ?? keys.at(-1);
  if (picked === undefined) {
    throw new Error('Nothing to pick from: no weights.');
  }
  return picked;
};

/** One of the items, each as likely. */
export const pickOne = <Item>(items: readonly Item[], random: Random): Item | undefined =>
  items[Math.floor(random() * items.length)];
