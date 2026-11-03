import type { IconName } from '../lib/icons';

/** How the app shows a tool: its icon, its name, what it asks before running, and what the agent does with it. */
export interface ToolLabel {
  icon: IconName;
  name: string;
  question: string;
  /** While the model writes the call, its arguments not known yet: "Writing a file". */
  writing: string;
  /** Before the call's summary while it runs: "Reading" src/greet.ts. */
  doing: string;
}
