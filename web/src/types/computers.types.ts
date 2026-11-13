import type { ComputerColor, DesktopRequest } from '../../../shared/protocol/desktop.schema';

/** Another computer's swcode as this browser keeps it: where it answers, and the token its pairing gave. */
export interface StoredComputer {
  address: string;
  color?: ComputerColor | undefined;
  id: string;
  name: string;
  token: string;
}

/** The computers a browser knows beside the one that served the page, and the one it shows. */
export interface StoredComputers {
  computers: StoredComputer[];
  current: string;
}

/** What a page asks about the computers it shows: the desktop app answers in its window, the browser itself elsewhere. */
export type ComputerRequest = Exclude<DesktopRequest, { type: 'openOutside' }>;

/** Where another computer's swcode is, and the code that pairs with it. */
export interface PairingTarget {
  origin: string;
  code: string;
}
