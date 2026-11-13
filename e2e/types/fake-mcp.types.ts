/** An MCP server at an address, for the daemon to reach: one tool, greet, which says hello to a name. */
export interface FakeMcp {
  /** Where it answers, as the servers file names it. */
  url: string;
  stop(): Promise<void>;
}
