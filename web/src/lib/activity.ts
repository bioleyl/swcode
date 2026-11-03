import { toolLabel } from './tool-labels';

import type { Activity, ActivityLines, ActivityMap } from '../types/activity.types';

const ELLIPSIS = '…';

const ACTIVITY_LINES: ActivityLines = {
  calling: (activity) => toolLabel(activity.tool).writing,
  running: (activity) => {
    const label = toolLabel(activity.tool);
    return activity.summary === '' ? label.writing : `${label.doing} ${activity.summary}`;
  },
  thinking: () => 'Thinking',
  working: () => 'Working',
  writing: () => 'Writing',
};

const lineOf = <Kind extends keyof ActivityMap>(kind: Kind, activity: ActivityMap[Kind]): string =>
  ACTIVITY_LINES[kind](activity);

/** The line under the conversation while the agent works, in a few words: "Reading src/greet.ts…". */
export const activityLine = (activity: Activity): string => `${lineOf(activity.kind, activity)}${ELLIPSIS}`;
