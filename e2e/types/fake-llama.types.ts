export interface ScriptedToolCall {
  name: string;
  /** The arguments as the model writes them: an object is sent as its JSON. */
  arguments: Record<string, unknown> | string;
}

/** One reply of the fake model: some text, some tool calls, or an error status instead. */
export interface ScriptedReply {
  text?: string;
  toolCalls?: readonly ScriptedToolCall[];
  failWith?: number;
  /** The error's type, with `failWith`: llama-server's own, such as exceed_context_size_error. */
  errorType?: string;
  /** What the model thinks before it answers, streamed apart from the answer. */
  reasoning?: string;
  /** How long the model thinks before it answers, to see work in progress. */
  delayMs?: number;
  /** A pause between the chunks, to see the reply stream in. */
  chunkDelayMs?: number;
}

/** A model a router serves, with its own settings. */
export interface FakeRouterModel {
  id: string;
  loaded: boolean;
  contextTokens: number;
  slots: number;
}

export interface FakeLlamaOptions {
  /** The replies, in order: each chat completion takes the next one. */
  replies: readonly ScriptedReply[];
  /**
   * Replies for the conversations whose first user message holds the key, each in order: requests that run at
   * the same time (subagents) cannot share one script.
   */
  scripts?: Readonly<Record<string, readonly ScriptedReply[]>>;
  models?: readonly string[];
  contextTokens?: number;
  /** The slots /props reports; left out, it does not say, like an older llama-server. */
  slots?: number;
  /** Whether /props says the model sees pictures, as with a vision projector loaded. */
  vision?: boolean;
  /** Answer as a router serving these models: each has its own /props, and requests must name one. */
  router?: readonly FakeRouterModel[];
}

export interface FakeLlama {
  /** Where the server answers, without /v1, as LLAMA_URL takes it. */
  url: string;
  /** The body of every chat completion request, in order. */
  requests: Record<string, unknown>[];
  /** The model each /props request named, in order; '' for none. */
  propsAsked: string[];
  stop(): Promise<void>;
}

/** A message of a chat completion request, as the fake llama-server received it: text, or text and pictures. */
export interface WireMessage {
  role: string;
  // biome-ignore lint/style/useNamingConvention: the Chat Completions API's own field
  content: string | { type: string; text?: string; image_url?: { url: string } }[];
}
