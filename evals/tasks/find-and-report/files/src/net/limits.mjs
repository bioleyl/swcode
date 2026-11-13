import { RETRY_TIMEOUT_MS } from '../config/defaults.mjs';

export const MAX_RETRIES = 3;
export const REQUEST_TIMEOUT_MS = RETRY_TIMEOUT_MS * 15;
