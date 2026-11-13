import { describe, expect, it } from 'vitest';

import {
  DEFAULT_WEEKDAY,
  EVERY_LABELS,
  everyOptions,
  isScheduleTime,
  scheduleLine,
  scheduleWhen,
  weekdayOf,
  weekdayOptions,
} from '../src/lib/schedules';

import type { ProjectSchedule } from '../../shared/protocol/project.schema';

const schedule = (changes: Partial<ProjectSchedule>): ProjectSchedule => ({
  at: '08:00',
  every: 'day',
  id: 's1',
  on: true,
  prompt: 'Run the tests',
  since: '2026-11-13T06:00:00.000Z',
  weekday: 0,
  ...changes,
});

describe('a schedule', () => {
  it('says when it runs, by how often', () => {
    expect(scheduleWhen(schedule({}))).toBe('Every day at 08:00');
    expect(scheduleWhen(schedule({ at: '00:30', every: 'hour' }))).toBe('Every hour at :30');
    expect(scheduleWhen(schedule({ at: '17:45', every: 'week', weekday: 5 }))).toBe('Every Friday at 17:45');
    // A day that is none is read as Sunday rather than break the row.
    expect(scheduleWhen(schedule({ every: 'week', weekday: 9 }))).toBe('Every Sunday at 08:00');
  });

  it('says when it last ran, or that it never did', () => {
    const now = new Date('2026-11-13T10:00:00.000Z');
    expect(scheduleLine(schedule({}), now)).toBe('Every day at 08:00 · Not run yet');
    expect(scheduleLine(schedule({ lastRunAt: '2026-11-13T07:00:00.000Z' }), now)).toBe(
      'Every day at 08:00 · last ran 3 h ago'
    );
  });

  it('is added from choices of how often and of a day', () => {
    expect(everyOptions().map((option) => option.value)).toEqual(['hour', 'day', 'week']);
    expect(everyOptions()[1]?.label).toBe(EVERY_LABELS.day);
    expect(weekdayOptions()).toHaveLength(7);
    expect(weekdayOptions()[1]).toEqual({ label: 'Monday', value: '1' });
    expect(weekdayOf('5')).toBe(5);
    expect(weekdayOf('')).toBe(DEFAULT_WEEKDAY);
    expect(weekdayOf('7')).toBe(DEFAULT_WEEKDAY);
  });

  it('takes a time of the day only', () => {
    expect(isScheduleTime('08:00')).toBe(true);
    expect(isScheduleTime('23:59')).toBe(true);
    expect(isScheduleTime('24:00')).toBe(false);
    expect(isScheduleTime('8:00')).toBe(false);
    expect(isScheduleTime('')).toBe(false);
  });
});
