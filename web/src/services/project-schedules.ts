import { daemon } from './daemon';

import type { ScheduleEvery } from '../../../shared/protocol/project.schema';
import type { NewSchedule } from '../types/schedules.types';

/**
 * Has the project start a session by itself on a prompt, every hour, day or week at a time. The daemon sends the
 * projects back changed.
 */
export const addProjectSchedule = (projectId: string, schedule: NewSchedule): void => {
  const every: ScheduleEvery = schedule.every;
  daemon.send({
    at: schedule.at,
    every,
    projectId,
    prompt: schedule.prompt,
    type: 'addProjectSchedule',
    ...(every === 'week' ? { weekday: schedule.weekday } : {}),
  });
};

/** Turns a schedule on or off: turned on, the times that passed while it was off are not made up for. */
export const setProjectScheduleOn = (projectId: string, scheduleId: string, on: boolean): void =>
  daemon.send({ on, projectId, scheduleId, type: 'setProjectScheduleOn' });

/** Removes a schedule; the sessions it started stay. */
export const removeProjectSchedule = (projectId: string, scheduleId: string): void =>
  daemon.send({ projectId, scheduleId, type: 'removeProjectSchedule' });
