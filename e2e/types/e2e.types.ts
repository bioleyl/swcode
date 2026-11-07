import type { Browser } from 'playwright-core';
import type { SessionEvent } from '../../shared/protocol/session.schema.js';
import type { FakeLlama, ScriptedReply } from './fake-llama.types.js';
import type { FakeRelease } from './fake-releases.types.js';

/** The Go daemon with the built web app, a fake llm-server, and a browser, for one test. */
export interface E2eRig {
  llama: FakeLlama;
  browser: Browser;
  workspace: string;
  /** The link that opens the app with its token. */
  link: string;
  /** The program the daemon runs: an update installed replaces it. */
  program: string;
  /** The events of the daemon's sessions, read from its data folder. */
  events(): SessionEvent[];
  stop(): Promise<void>;
}

export interface E2eOptions {
  replies: readonly ScriptedReply[];
  /** How many requests the model answers at once, 2 when left out. */
  slots?: number;
  /** Replies for the conversations whose first message says the key: a subagent's task. */
  scripts?: Readonly<Record<string, readonly ScriptedReply[]>>;
  files?: Readonly<Record<string, string>>;
  /** Files of the daemon's home folder, by path: the person's skills in .agents/skills. */
  homeFiles?: Readonly<Record<string, string>>;
  /** The model llm-server serves, gpt-oss-120b when left out. */
  model?: string;
  /** Whether that model sees pictures. */
  vision?: boolean;
  /** A newer release on the fake GitHub: the daemon then runs a copy of its program, which installing replaces. */
  release?: FakeRelease;
}
