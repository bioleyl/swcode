/** What one ask at another swcode gives: its token for the number it waits for, or why not, and whether to ask again. */
export type PairAsk =
  | { ok: true; token: string }
  | { ok: false; problem: string; retry: boolean; waitMs?: number };
