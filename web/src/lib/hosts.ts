import { hostLabel } from '../../../shared/hosts/host-key';

import type { HostRef } from '../../../shared/protocol/project.schema';

type HostKind = HostRef['kind'];

// How each kind of host shows beside its projects: a project is a folder, on this computer as in a WSL distribution
// (another file system of the same one), and a server elsewhere.
export const HOST_ICONS: Readonly<Record<HostKind, string>> = { local: 'folder', ssh: 'server', wsl: 'folder' };

/** A folder as people read it: on this computer its path, elsewhere the host first. */
export const placeText = (host: HostRef, path: string): string =>
  host.kind === 'local' ? path : `${hostLabel(host)} · ${path}`;
