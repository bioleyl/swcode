import { describe, expect, it } from 'vitest';

import { MAX_DRAFTS, withDraft } from '../src/lib/drafts';

describe('withDraft', () => {
  it('keeps what was typed for a place, beside the other places', () => {
    expect(withDraft({ a: 'first' }, 'b', 'second')).toEqual({ a: 'first', b: 'second' });
  });

  it('replaces a place’s text, which becomes the latest written', () => {
    expect(Object.keys(withDraft({ a: 'old', b: 'other' }, 'a', 'new'))).toEqual(['b', 'a']);
  });

  it('forgets a place once its text is empty', () => {
    expect(withDraft({ a: 'first', b: 'second' }, 'a', '')).toEqual({ b: 'second' });
  });

  it('keeps exactly the most it may, then drops the one written longest ago', () => {
    const full = Object.fromEntries(Array.from({ length: MAX_DRAFTS }, (_, index) => [`s${index}`, 'x']));
    expect(Object.keys(withDraft(full, 's0', 'again'))).toHaveLength(MAX_DRAFTS);
    const past = withDraft(full, 'one-more', 'x');
    expect(Object.keys(past)).toHaveLength(MAX_DRAFTS);
    expect(past.s0).toBeUndefined();
    expect(past['one-more']).toBe('x');
  });
});
