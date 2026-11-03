const SEPARATOR = '/';

/** A folder inside another, on the daemon's computer. */
export const childPath = (parent: string, name: string): string =>
  parent.endsWith(SEPARATOR) ? `${parent}${name}` : `${parent}${SEPARATOR}${name}`;
