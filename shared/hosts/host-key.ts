import type { HostRef } from '../protocol/project.schema.js';

type HostKind = HostRef['kind'];

// One string per host, to tell hosts apart and to know a project by its host and folder.
const KEYS: { readonly [Kind in HostKind]: (ref: Extract<HostRef, { kind: Kind }>) => string } = {
  local: () => 'local',
  ssh: (ref) => `ssh:${ref.endpoint}`,
  wsl: (ref) => `wsl:${ref.distro}`,
};

export const hostKey = (ref: HostRef): string => (KEYS[ref.kind] as (ref: HostRef) => string)(ref);

export const sameHost = (first: HostRef, second: HostRef): boolean => hostKey(first) === hostKey(second);

// How each host reads for people.
const LABELS: { readonly [Kind in HostKind]: (ref: Extract<HostRef, { kind: Kind }>) => string } = {
  local: () => 'This computer',
  ssh: (ref) => ref.endpoint,
  wsl: (ref) => `WSL · ${ref.distro}`,
};

export const hostLabel = (ref: HostRef): string => (LABELS[ref.kind] as (ref: HostRef) => string)(ref);
