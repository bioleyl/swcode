import { css } from '@jadis/core';
import { JlDialog, JlIcon, JlKbd } from '@syware/jlib';

import { ScElement } from '../base/sc-element';
import { COMMAND_PREFIX, COMMANDS } from '../lib/commands';
import { BASE_STYLES } from '../styles/shared';
import styles from './sc-command-help.css?inline';

import type { CommandName } from '../lib/commands';
import type { FieldKey } from '../types/commands.types';

const ICON_SIZE = 16;
/** The keys of the message field, as `jl-kbd` names them, in the order people meet them. */
const FIELD_KEYS: readonly FieldKey[] = [
  { does: 'Send, or run the command chosen', keys: ['enter'] },
  { does: 'Start a new line', keys: ['shift+enter'] },
  { does: 'Move through the commands listed', keys: ['up', 'down'] },
  { does: 'Complete the command chosen', keys: ['tab'] },
  { does: 'Close the list', keys: ['esc'] },
];

/** What /help shows: the commands the composer takes here, and the keys of its field. */
export class ScCommandHelp extends ScElement {
  static readonly selector = 'sc-command-help';

  /** The commands available where the composer is: a chat has no /peek. */
  readonly commands = this.useChange<CommandName[]>([], (names) => this.renderCommands(names));

  private readonly _refs = this.useRefs((ref) => ({
    commands: ref('.commands'),
    dialog: ref(JlDialog),
  }));

  templateHtml(): Node {
    return (
      <JlDialog heading="Commands" description="Type / at the start of a message to list them.">
        <dl class="commands" />
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
