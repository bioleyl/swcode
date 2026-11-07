import { authorizationHeaders } from './access';

const JSON_TYPE = { 'content-type': 'application/json' };

/** The answer's JSON, or nothing when the daemon did not answer or not with JSON. */
export const jsonOf = (response: Response | undefined): Promise<unknown> =>
  response ? response.json().catch(() => undefined) : Promise.resolve(undefined);

/** The answer's body as text, for a refusal whose words the app shows. */
export const textOf = (response: Response): Promise<string> => response.text().catch(() => '');

/** The JSON of a body already read as text; nothing when it is not JSON. */
export const jsonOfText = (text: string): unknown => {
  try {
    return JSON.parse(text);
  } catch {
    return undefined;
  }
};

/** A call to the daemon's API, with the access token; nothing when the daemon does not answer. */
export const request = (method: string, path: string, body?: unknown): Promise<Response | undefined> =>
  fetch(path, {
    headers: { ...authorizationHeaders(), ...(body === undefined ? {} : JSON_TYPE) },
    method,
    ...(body === undefined ? {} : { body: JSON.stringify(body) }),
  }).catch(() => undefined);
