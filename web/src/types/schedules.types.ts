import type { ScheduleEvery } from '../../../shared/protocol/project.schema';

/** A schedule as the form holds it before the daemon keeps it: its prompt, how often, at what time, on which day. */
export interface NewSchedule {
  prompt: string;
  every: ScheduleEvery;
  /** A time of the day, `08:00`. */
  at: string;
  /** The day of the week, Sunday being 0: counts for a schedule that runs every week. */
  weekday: number;
}
