import assert from 'node:assert/strict';
import { test } from 'node:test';

import { sum } from './sum.mjs';

test('adds the numbers', () => {
  assert.equal(sum([1, 2, 3]), 6);
});

test('is 0 for none', () => {
  assert.equal(sum([]), 0);
});
