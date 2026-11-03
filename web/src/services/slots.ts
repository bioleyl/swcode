import { Bus } from '@jadis/core';

import { daemonBus } from './daemon';

import type { SlotUsage } from '../../../shared/protocol/messages.schema';

// The slots live as long as the page: their bus listener never needs to stop.
const forever = new AbortController().signal;

/** How busy each model's slots are, as the daemon last said. */
export const slotsBus = new Bus<{ changed: readonly SlotUsage[] }>();

const state = { models: [] as SlotUsage[] };

export const slotUsage = (): readonly SlotUsage[] => state.models;

daemonBus.register(
  'message',
  (message) => {
    if (message.type !== 'slots') {
      return;
    }
    state.models = message.models;
    slotsBus.emit('changed', message.models);
  },
  forever
);
