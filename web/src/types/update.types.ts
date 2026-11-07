import type { Tone } from '@syware/jlib';
import type { UpdateState } from '../../../shared/protocol/update.schema';

export type UpdateStateMap = { [State in UpdateState as State['kind']]: State };

/** What the Updates topic says beside its facts, in an alert of that tone. */
export interface UpdateNote {
  text: string;
  tone: Tone;
}
