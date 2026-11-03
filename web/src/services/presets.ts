import { Bus } from '@jadis/core';

import { NO_PRESETS } from '../lib/presets';
import { daemonBus } from './daemon';

import type { PresetChoices } from '../../../shared/protocol/models.schema';

// The presets live as long as the page: their bus listener never needs to stop.
const forever = new AbortController().signal;

/** The presets as the daemon last listed them: it says them at connection, and again when they change. */
export const presetsBus = new Bus<{ changed: undefined }>();

const state = { choices: NO_PRESETS };

export const presetChoices = (): PresetChoices => state.choices;

daemonBus.register(
  'message',
  (message) => {
    if (message.type !== 'presets') {
      return;
    }
    state.choices = message.choices;
    presetsBus.emit('changed');
  },
  forever
);
