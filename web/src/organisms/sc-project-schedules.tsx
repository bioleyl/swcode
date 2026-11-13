import { createElement, css } from '@jadis/core';
import { JlButton, JlCard, JlField, JlInput, JlLink, JlListItem, JlSelect, JlSwitch } from '@syware/jlib';

import { MAX_SCHEDULE_PROMPT } from '../../../shared/protocol/project.schema';
import { ScElement } from '../base/sc-element';
import {
  DEFAULT_EVERY,
  DEFAULT_TIME,
  DEFAULT_WEEKDAY,
  everyOptions,
  isScheduleTime,
  scheduleLine,
  weekdayOf,
  weekdayOptions,
} from '../lib/schedules';
import { hrefOf, sessionPath } from '../services/navigation';
import { addProjectSchedule, removeProjectSchedule, setProjectScheduleOn } from '../services/project-schedules';
import { BASE_STYLES } from '../styles/shared';
import styles from './sc-project-schedules.css?inline';

import type { Project, ProjectSchedule, ScheduleEvery } from '../../../shared/protocol/project.schema';

const NOTE =
  'Sessions this project starts by itself: each on its prompt, every hour, day or week at its time (the daemon’s own), with what the project’s new sessions start with. A time that passed while swcode was off runs once when it is back.';
const NONE = 'No schedule yet.';
const PROMPT_PLACEHOLDER = 'Run the tests and fix what fails';

/**
 * A project's schedules, in its Config tab: the sessions it starts by itself, each with when it runs and when it
 * last did, a switch that turns it off, a link to the session it last started and its removal; under them, the form
 * that adds one.
 */
export class ScProjectSchedules extends ScElement {
  static readonly selector = 'sc-project-schedules';

  readonly project = this.useChange<Project | undefined>(undefined, () => this.render());

  // What the rows show: the list is drawn again only when it changes, so a switch keeps its focus.
  private _shown = '';

  private readonly _refs = this.useRefs((ref) => ({
    at: ref<JlInput>('.at'),
    day: ref<JlField>('.day'),
    every: ref<JlSelect<ScheduleEvery>>('.every'),
    form: ref<HTMLFormElement>('.add'),
    none: ref('.none'),
    prompt: ref<JlInput>('.prompt'),
    rows: ref('.rows'),
    weekday: ref<JlSelect>('.weekday'),
  }));

  templateHtml(): Node {
    return (
      <JlCard class="schedules" heading="Schedules" description={NOTE}>
        <div class="rows" />
        <p class="none">{NONE}</p>
        <form class="add">
          <JlField label="Prompt">
            <JlInput
              class="prompt"
              autocomplete="off"
              placeholder={PROMPT_PLACEHOLDER}
              maxlength={String(MAX_SCHEDULE_PROMPT)}
            />
          </JlField>
          <div class="when">
            <JlField label="Runs">
              <JlSelect class="every" />
            </JlField>
            <JlField class="day" label="On" hidden>
              <JlSelect class="weekday" />
            </JlField>
            <JlField label="At">
              <JlInput class="at" type="time" />
            </JlField>
            <JlButton type="submit" icon="plus" label="Add" />
          </div>
        </form>
      </JlCard>
    );
  }

  templateCss(): string {
    return css`${BASE_STYLES}${styles}`;
  }

  onConnect(): void {
    this._refs.every.options.set(everyOptions());
    this._refs.every.value.set(DEFAULT_EVERY);
    this._refs.weekday.options.set(weekdayOptions());
    this._refs.weekday.value.set(String(DEFAULT_WEEKDAY));
    this._refs.at.value.set(DEFAULT_TIME);
    // A day counts for a schedule that runs every week only.
    this._refs.every.events.register('change', (every) => {
      this._refs.day.hidden = every !== 'week';
    });
    this.on(this._refs.form, 'submit', (event) => {
      event.preventDefault();
      this.add();
    });
    this.render();
  }

  /** The schedule the form holds joins the project, and the form is ready for the next. */
  private add(): void {
    const project = this.project.get();
    const prompt = this._refs.prompt.value.get().trim();
    const at = this._refs.at.value.get();
    const every = this._refs.every.value.get();
    if (!project || prompt === '' || every === '' || !isScheduleTime(at)) {
      return;
    }
    addProjectSchedule(project.id, { at, every, prompt, weekday: weekdayOf(this._refs.weekday.value.get()) });
    this._refs.prompt.value.set('');
  }

  private render(): void {
    const project = this.project.get();
    if (!project || !this.isConnected) {
      return;
    }
    const schedules = project.schedules ?? [];
    const shown = `${project.id}\n${JSON.stringify(schedules)}`;
    if (shown === this._shown) {
      return;
    }
    this._shown = shown;
    const now = new Date();
    this._refs.rows.replaceChildren(...schedules.map((schedule) => this.rowNode(project.id, schedule, now)));
    this._refs.rows.hidden = schedules.length === 0;
    this._refs.none.hidden = schedules.length > 0;
  }

  /**
   * A schedule and its own controls: jlib's events do not bubble, so each switch and each button knows its schedule.
   */
  private rowNode(projectId: string, schedule: ProjectSchedule, now: Date): Node {
    const end = createElement('div', { attrs: { class: 'end', slot: 'end' } });
    if (schedule.lastSessionId !== undefined && schedule.lastSessionId !== '') {
      end.append(
        createElement(JlLink, {
          attrs: { class: 'last' },
          props: { href: hrefOf(sessionPath(schedule.lastSessionId)), label: 'Last session' },
        })
      );
    }
    const toggle = createElement(JlSwitch, {
      attrs: { class: 'on' },
      props: { checked: schedule.on, label: schedule.on ? 'On' : 'Off' },
    });
    toggle.events.register('change', (on) => setProjectScheduleOn(projectId, schedule.id, on));
    const remove = createElement(JlButton, {
      attrs: { class: 'remove' },
      props: { hint: 'Remove this schedule', icon: 'trash', size: 'sm', variant: 'ghost' },
    });
    remove.events.register('press', () => removeProjectSchedule(projectId, schedule.id));
    end.append(toggle, remove);
    const item = createElement(JlListItem, {
      attrs: { class: 'schedule' },
      props: { description: scheduleLine(schedule, now), heading: schedule.prompt },
    });
    item.append(end);
    return item;
  }
}

ScProjectSchedules.register();
