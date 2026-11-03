/** biome-ignore-all lint/style/useNamingConvention: llama-server answers in snake_case (finish_reason, n_ctx) */
import { createServer } from 'node:http';
import { setTimeout as sleep } from 'node:timers/promises';

import type { IncomingMessage, ServerResponse } from 'node:http';
import type { FakeLlama, FakeLlamaOptions, FakeRouterModel, ScriptedReply } from './types/fake-llama.types.js';

const HTTP = { badRequest: 400, notFound: 404, ok: 200 } as const;
const NO_MORE_REPLIES = 'The fake model has no more replies.';
const NO_MODEL = 'model name is missing from the request';
const MODEL_STATUS = { false: 'unloaded', true: 'loaded' } as const;
const LOCAL_BASE = 'http://127.0.0.1';
const COMPLETION_TOKENS = 40;
// Its token count: four characters a token of the messages, close enough for tests of the context budget.
const CHARS_PER_TOKEN = 4;

const promptTokensOf = (body: Record<string, unknown>): number =>
  Math.ceil(JSON.stringify(body.messages ?? []).length / CHARS_PER_TOKEN);

const readBody = async (request: IncomingMessage): Promise<Record<string, unknown>> => {
  const chunks: Buffer[] = [];
  for await (const chunk of request) {
    chunks.push(Buffer.from(chunk));
  }
  const text = Buffer.concat(chunks).toString('utf8');
  return text ? JSON.parse(text) : {};
};

const answer = (response: ServerResponse, status: number, body: unknown): void => {
  response.writeHead(status, { 'content-type': 'application/json' }).end(JSON.stringify(body));
};

const halves = (text: string): string[] => {
  const middle = Math.ceil(text.length / 2);
  return [text.slice(0, middle), text.slice(middle)].filter((part) => part !== '');
};

const deltaChunk = (delta: Record<string, unknown>) => ({
  choices: [{ delta, index: 0 }],
  object: 'chat.completion.chunk',
});

/** The chunks a streaming reply arrives in: text in pieces, each tool call named first, then its arguments. */
const chunksOf = (reply: ScriptedReply, promptTokens: number): unknown[] => [
  ...halves(reply.reasoning ?? '').map((reasoning) => deltaChunk({ reasoning_content: reasoning })),
  ...halves(reply.text ?? '').map((content) => deltaChunk({ content })),
  ...(reply.toolCalls ?? []).flatMap((call, index) => {
    const text = typeof call.arguments === 'string' ? call.arguments : JSON.stringify(call.arguments);
    return [
      deltaChunk({ tool_calls: [{ function: { arguments: '', name: call.name }, id: `call-${index}`, index }] }),
      ...halves(text).map((part) => deltaChunk({ tool_calls: [{ function: { arguments: part }, index }] })),
    ];
  }),
  { choices: [{ delta: {}, finish_reason: reply.toolCalls ? 'tool_calls' : 'stop', index: 0 }] },
  { choices: [], usage: { completion_tokens: COMPLETION_TOKENS, prompt_tokens: promptTokens } },
];

async function stream(response: ServerResponse, reply: ScriptedReply, promptTokens: number): Promise<void> {
  response.writeHead(HTTP.ok, { 'content-type': 'text/event-stream' });
  for (const chunk of chunksOf(reply, promptTokens)) {
    response.write(`data: ${JSON.stringify(chunk)}\n\n`);
    await sleep(reply.chunkDelayMs ?? 0);
  }
  response.end('data: [DONE]\n\n');
}

/** A message's text: all of it, or its text parts when it carries pictures too. */
const textOf = (content: unknown): string => {
  if (typeof content === 'string') {
    return content;
  }
  return Array.isArray(content)
    ? content.map((part) => (typeof part?.text === 'string' ? part.text : '')).join('\n')
    : '';
};

const firstUserMessage = (body: Record<string, unknown>): string => {
  const messages = Array.isArray(body.messages) ? body.messages : [];
  return textOf(messages.find((message) => message?.role === 'user')?.content);
};

const routerModels = (router: readonly FakeRouterModel[]) => ({
  data: router.map((model) => ({
    id: model.id,
    object: 'model',
    status: { value: MODEL_STATUS[`${model.loaded}`] },
  })),
});

/** A router's /props: the asked model's settings, or the router's own, which have no context. */
const routerProps = (router: readonly FakeRouterModel[], path: string) => {
  const asked = router.find((model) => model.id === new URL(path, LOCAL_BASE).searchParams.get('model'));
  return asked
    ? { default_generation_settings: { n_ctx: asked.contextTokens }, total_slots: asked.slots }
    : { default_generation_settings: { n_ctx: 0 }, role: 'router' };
};

/** A llama-server stand-in: streams the replies a test scripts, and records what the agent sent it. */
export async function startFakeLlama(options: FakeLlamaOptions): Promise<FakeLlama> {
  const requests: Record<string, unknown>[] = [];
  const propsAsked: string[] = [];
  const replies = [...options.replies];
  const scripts = Object.entries(options.scripts ?? {}).map(([key, script]) => ({ key, replies: [...script] }));
  const nextReply = (body: Record<string, unknown>): ScriptedReply | undefined => {
    const script = scripts.find((candidate) => firstUserMessage(body).includes(candidate.key));
    return (script ? script.replies : replies).shift();
  };
  const server = createServer(async (request, response) => {
    const path = request.url ?? '';
    const body = await readBody(request);
    if (path === '/v1/models' && options.router) {
      answer(response, HTTP.ok, routerModels(options.router));
      return;
    }
    if (path === '/v1/models') {
      answer(response, HTTP.ok, { data: (options.models ?? []).map((id) => ({ id, object: 'model' })) });
      return;
    }
    if (path.startsWith('/props')) {
      propsAsked.push(new URL(path, LOCAL_BASE).searchParams.get('model') ?? '');
    }
    if (path.startsWith('/props') && options.router) {
      answer(response, HTTP.ok, routerProps(options.router, path));
      return;
    }
    if (path.startsWith('/props')) {
      answer(response, HTTP.ok, {
        default_generation_settings: { n_ctx: options.contextTokens },
        modalities: { vision: options.vision ?? false },
        ...(options.slots === undefined ? {} : { total_slots: options.slots }),
      });
      return;
    }
    if (path !== '/v1/chat/completions') {
      answer(response, HTTP.notFound, { error: { message: 'Not found' } });
      return;
    }
    requests.push(body);
    if (options.router && typeof body.model !== 'string') {
      answer(response, HTTP.badRequest, { error: { message: NO_MODEL, type: 'invalid_request_error' } });
      return;
    }
    const reply = nextReply(body) ?? { failWith: HTTP.notFound, text: NO_MORE_REPLIES };
    await sleep(reply.delayMs ?? 0);
    if (reply.failWith) {
      answer(response, reply.failWith, {
        error: { message: reply.text ?? 'Refused by the fake server', type: reply.errorType },
      });
      return;
    }
    await stream(response, reply, promptTokensOf(body));
  });
  await new Promise<void>((resolve) => server.listen(0, '127.0.0.1', resolve));
  const address = server.address();
  if (address === null || typeof address === 'string') {
    throw new Error('The fake llama-server has no port.');
  }
  return {
    propsAsked,
    requests,
    stop: () => new Promise<void>((resolve) => server.close(() => resolve())),
    url: `http://127.0.0.1:${address.port}`,
  };
}
