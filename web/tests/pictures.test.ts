import { describe, expect, it } from 'vitest';

import { fittedSize, MAX_PICTURE_SIDE } from '../src/lib/pictures';

describe('fittedSize', () => {
  it('brings the longest side down to the limit, keeping the shape', () => {
    expect(fittedSize(3840, 2160)).toEqual({ height: 720, width: MAX_PICTURE_SIDE });
    expect(fittedSize(1000, 4000)).toEqual({ height: MAX_PICTURE_SIDE, width: 320 });
  });

  it('leaves a picture that fits as it is', () => {
    expect(fittedSize(800, 600)).toEqual({ height: 600, width: 800 });
  });
});
