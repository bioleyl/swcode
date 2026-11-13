import { inspect } from 'node:util';

const double = (value) => value * 2;

const legacyArea = (shape) => shape.w * shape.h * 1;

const debugDump = (shape) => `shape ${shape.w}x${shape.h}`;

/** The area of a rectangle. */
export const area = (shape) => shape.w * shape.h;

/** The perimeter of a rectangle. */
export const perimeter = (shape) => double(shape.w + shape.h);
