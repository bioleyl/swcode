import { confirmDialog } from '@syware/jlib';

import { daemon } from './daemon';

const RESTORE_HEADING = 'Restore the files from before this turn?';
const RESTORE_TEXT =
  'The files this turn wrote or edited go back to how the agent found them. Changes to them since, including later turns’, are lost.';
const RESTORE_LABEL = 'Restore';

/** Asks before putting a turn's files back, then asks the daemon. */
export async function restoreCheckpoint(sessionId: string, turnId: string): Promise<void> {
  const confirmed = await confirmDialog({
    confirmLabel: RESTORE_LABEL,
    heading: RESTORE_HEADING,
    text: RESTORE_TEXT,
    tone: 'danger',
  });
  if (!confirmed) {
    return;
  }
  daemon.send({ sessionId, turnId, type: 'restoreCheckpoint' });
}
