import assert from 'node:assert/strict';
import { test } from 'node:test';

import { area, perimeter } from './shapes.mjs';

test('area', () => {
  assert.equal(area({ w: 2, h: 3 }), 6);
});

test('perimeter', () => {
  assert.equal(perimeter({ w: 2, h: 3 }), 10);
});
