import { confirmDialog } from '@syware/jlib';

import { daemon } from './daemon';

import type { SessionKind, SessionSummary } from '../../../shared/protocol/session.schema';

/** What deleting a session takes, said in the dialog that asks first: a coding session leaves its project behind. */
const DELETE_CONSEQUENCES: Readonly<Record<SessionKind, string>> = {
  chat: 'Its conversation and its pictures go for good.',
  code: 'Its conversation goes for good. Its project and files stay.',
};
const WORKFLOW_RUN_CONSEQUENCE =
  'Its conversation, its run and every step session under it go for good. Its project and files stay.';
const consequenceOf = (session: SessionSummary): string =>
  session.workflowRunId !== undefined && session.workflowStepId === undefined
    ? WORKFLOW_RUN_CONSEQUENCE
    : DELETE_CONSEQUENCES[session.kind];

/** Deletes a session once the person confirms, its conversation for good: whether it goes. */
export async function deleteSession(session: SessionSummary): Promise<boolean> {
  const confirmed = await confirmDialog({
    confirmLabel: 'Delete',
    heading: `Delete “${session.title}”?`,
    text: consequenceOf(session),
    tone: 'danger',
  });
  if (!confirmed) {
    return false;
  }
  daemon.send({ sessionId: session.id, type: 'deleteSession' });
  return true;
}
