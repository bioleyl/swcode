import { JlElement } from '@syware/jlib';

/**
 * The base of every component of swcode's own: jlib's, which gives the document jlib's tokens when the first one
 * registers. Inputs are `useChange` fields: bound to an attribute for text, numbers and flags
 * (`{ attribute: 'label' }`), or set as properties for lists and objects.
 */
export abstract class ScElement extends JlElement {}

/** An attribute's value when it is one of the allowed ones, otherwise the fallback. */
export const oneOf =
  <Value extends string>(allowed: readonly Value[], fallback: Value) =>
  (value: string | null): Value =>
    allowed.find((candidate) => candidate === value) ?? fallback;
