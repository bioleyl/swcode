import { css } from '@jadis/core';
import { JlDialog, JlIcon, JlKbd } from '@syware/jlib';

import { ScElement } from '../base/sc-element';
import { COMMAND_PREFIX, COMMANDS } from '../lib/commands';
import { BASE_STYLES } from '../styles/shared';
import styles from './sc-command-help.css?inline';

import type { SkillSummary } from '../../../shared/protocol/skill.schema';
import type { CommandName } from '../lib/commands';
import type { FieldKey } from '../types/commands.types';

const ICON_SIZE = 16;
const SKILL_ICON = 'book-open';
/** The keys of the message field, as `jl-kbd` names them, in the order people meet them. */
const FIELD_KEYS: readonly FieldKey[] = [
  { does: 'Send, or run the command chosen', keys: ['enter'] },
  { does: 'Start a new line', keys: ['shift+enter'] },
  { does: 'Move through the commands listed', keys: ['up', 'down'] },
  { does: 'Complete the command chosen', keys: ['tab'] },
  { does: 'Close the list', keys: ['esc'] },
];

/** What /help shows: the commands the composer takes here, the skills it runs, and the keys of its field. */
export class ScCommandHelp extends ScElement {
  static readonly selector = 'sc-command-help';

  /** The commands available where the composer is: a chat has no /peek. */
  readonly commands = this.useChange<CommandName[]>([], (names) => this.renderCommands(names));
  /** The skills here: each runs as a message, `/<its name>` and the words after it. */
  readonly skills = this.useChange<SkillSummary[]>([], (skills) => this.renderSkills(skills));

  private readonly _refs = this.useRefs((ref) => ({
    commands: ref('.commands'),
    dialog: ref(JlDialog),
    skills: ref('.skills'),
    skillsLabel: ref('.skills-label'),
  }));

  templateHtml(): Node {
    return (
      <JlDialog heading="Commands" description="Type / at the start of a message to list them." size="lg">
        <dl class="commands" />
        <h3 class="label skills-label" hidden>
          Skills
        </h3>
        <dl class="skills" hidden />
        <h3 class="label">Keys</h3>
        <dl class="keys">
          {FIELD_KEYS.map((key) => (
            <div class="row">
              <dt>
                {key.keys.map((keys) => (
                  <JlKbd keys={keys} />
                ))}
              </dt>
              <dd>{key.does}</dd>
            </div>
          ))}
        </dl>
      </JlDialog>
    );
  }

  templateCss(): string {
    return css`${BASE_STYLES}${styles}`;
  }

  show(): void {
    this._refs.dialog.show();
  }

  private renderSkills(skills: readonly SkillSummary[]): void {
    this._refs.skills.hidden = skills.length === 0;
    this._refs.skillsLabel.hidden = skills.length === 0;
    this._refs.skills.replaceChildren(
      ...skills.map((skill) => (
        <div class="row">
          <dt>
            <JlIcon name={SKILL_ICON} size={ICON_SIZE} />
            <code>
              {COMMAND_PREFIX}
              {skill.name}
            </code>
          </dt>
          <dd title={skill.description}>{skill.description}</dd>
        </div>
      ))
    );
  }

  private renderCommands(names: readonly CommandName[]): void {
    this._refs.commands.replaceChildren(
      ...names.map((name) => {
        const { argument, description, icon } = COMMANDS[name];
        return (
          <div class="row">
            <dt>
              <JlIcon name={icon} size={ICON_SIZE} />
              <code>
                {COMMAND_PREFIX}
                {name}
                {argument === undefined ? null : <span class="argument">{` ‹${argument}›`}</span>}
              </code>
            </dt>
            <dd>{description}</dd>
          </div>
        );
      })
    );
  }
}

ScCommandHelp.register();
