import { modelName } from './model-name';

import type { SlotUsage, Status } from '../../../shared/protocol/messages.schema';
import type { PresetStatus } from '../../../shared/protocol/models.schema';
import type { ModelCard } from '../types/conversation.types';

const UNNAMED = 'llama-server';

/** A single-model llama-server's model, which its slots do not name: the one its source lists. */
const servedBy = (status: Status | undefined, source: string): string =>
  status?.llama.sources.find((each) => each.name === source)?.models[0] ?? UNNAMED;

/**
 * The models to show: the ones the daemon asked, with their slots and their llama-server, or the session's main
 * model before any request, when its llama-server answers.
 */
export function modelCards(
  slots: readonly SlotUsage[],
  status: Status | undefined,
  preset: PresetStatus | undefined
): ModelCard[] {
  if (slots.length === 0) {
    return preset?.model ? [{ busy: [], name: modelName(preset.model), own: [], total: undefined }] : [];
  }
  return slots.map((usage) => ({
    busy: usage.busy,
    name: modelName(usage.model || servedBy(status, usage.source)),
    own: usage.own,
    source: usage.source,
    total: usage.total,
  }));
}

/**
 * Which of the model's slots this daemon uses, numbered from 1, when it shares them with other daemons: nothing
 * when it has them all.
 */
export function shareText(own: readonly number[], total: number): string | undefined {
  const first = own[0];
  const last = own.at(-1);
  if (first === undefined || last === undefined || own.length === total) {
    return undefined;
  }
  const slots = first === last ? `slot ${first + 1}` : `slots ${first + 1}–${last + 1}`;
  return `Its share: ${slots} of ${total}`;
}
