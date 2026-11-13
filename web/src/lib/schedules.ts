import { ago } from './format';

import type { SelectOption } from '@syware/jlib';
import type { ProjectSchedule, ScheduleEvery } from '../../../shared/protocol/project.schema';

/** The days of the week by their number, Sunday being 0, as a schedule keeps its day. */
export const WEEKDAYS = ['Sunday', 'Monday', 'Tuesday', 'Wednesday', 'Thursday', 'Friday', 'Saturday'] as const;

/** A schedule's day when the person picks none: Monday. */
export const DEFAULT_WEEKDAY = 1;
/** A schedule's time when the person picks none. */
export const DEFAULT_TIME = '08:00';
export const DEFAULT_EVERY: ScheduleEvery = 'day';

const MINUTE_START = 3;
const NEVER_RAN = 'Not run yet';

/** How often a schedule runs, as the form's choices name it: a new one does not compile until it is here. */
export const EVERY_LABELS: Readonly<Record<ScheduleEvery, string>> = {
  day: 'Every day',
  hour: 'Every hour',
  week: 'Every week',
};

/** When a schedule runs, by how often: every hour its minute alone counts, every week its day is named. */
const WHEN: Readonly<Record<ScheduleEvery, (schedule: ProjectSchedule) => string>> = {
  day: (schedule) => `Every day at ${schedule.at}`,
  hour: (schedule) => `Every hour at :${schedule.at.slice(MINUTE_START)}`,
  week: (schedule) => `Every ${WEEKDAYS[schedule.weekday] ?? WEEKDAYS[0]} at ${schedule.at}`,
};

/** When a schedule runs, in words: "Every day at 08:00", "Every hour at :30", "Every Monday at 08:00". */
export const scheduleWhen = (schedule: ProjectSchedule): string => WHEN[schedule.every](schedule);

/** What a schedule's row says under its prompt: when it runs, then when it last did. */
export const scheduleLine = (schedule: ProjectSchedule, now: Date): string => {
  const ran = schedule.lastRunAt === undefined ? NEVER_RAN : `last ran ${ago(schedule.lastRunAt, now)}`;
  return `${scheduleWhen(schedule)} · ${ran}`;
};

/** The form's choices of how often, in the order hour, day, week. */
export const everyOptions = (): SelectOption<ScheduleEvery>[] => [
  { label: EVERY_LABELS.hour, value: 'hour' },
  { label: EVERY_LABELS.day, value: 'day' },
  { label: EVERY_LABELS.week, value: 'week' },
];

/** The form's choices of a day, by its number as text: a select's values are texts. */
export const weekdayOptions = (): SelectOption[] =>
  WEEKDAYS.map((label, index) => ({ label, value: String(index) }));

/** The day a select's value names; Monday when it names none. */
export const weekdayOf = (value: string): number => {
  const day = Number.parseInt(value, 10);
  return Number.isInteger(day) && day >= 0 && day < WEEKDAYS.length ? day : DEFAULT_WEEKDAY;
};

const TIME = /^([01]\d|2[0-3]):[0-5]\d$/;

/** Whether a text is a time of the day as a schedule keeps it, `08:00`. */
export const isScheduleTime = (value: string): boolean => TIME.test(value);
