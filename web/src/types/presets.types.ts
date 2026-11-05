/** A context size as typed: none (as the llm-server says), a number of tokens, or words that are none. */
export type TokensInput = { kind: 'none' } | { kind: 'tokens'; tokens: number } | { kind: 'invalid' };
