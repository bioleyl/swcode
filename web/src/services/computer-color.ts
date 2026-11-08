import type { ComputerColor } from '../../../shared/protocol/desktop.schema';

const COLOR_ATTRIBUTE = 'computerColor';

/** Makes the shown computer's colour the app's primary one, or puts the default one back with none. */
export function applyComputerColor(color: ComputerColor | undefined): void {
  if (color === undefined) {
    delete document.documentElement.dataset[COLOR_ATTRIBUTE];
    return;
  }
  document.documentElement.dataset[COLOR_ATTRIBUTE] = color;
}
