/** What a request to the daemon gives back: its answer, or why it did not come off. */
export type Answer<Value> = { ok: true; answer: Value } | { ok: false; problem: string };
