import type { z } from 'zod';

/** A value kept in this browser, or the fallback when storage is off, empty, or holds something else. */
export function readStored<Value>(key: string, schema: z.ZodType<Value>, fallback: Value): Value {
  try {
    const text = localStorage.getItem(key);
    if (text === null) {
      return fallback;
    }
    const parsed = schema.safeParse(JSON.parse(text));
    return parsed.success ? parsed.data : fallback;
  } catch {
    return fallback;
  }
}

/** Keeps a value in this browser; without storage, it lasts as long as whoever holds it. */
export function writeStored(key: string, value: unknown): void {
  try {
    localStorage.setItem(key, JSON.stringify(value));
  } catch {
    // Storage is off: the caller keeps the value in memory.
  }
}
