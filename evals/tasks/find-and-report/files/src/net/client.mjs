import { MAX_RETRIES, REQUEST_TIMEOUT_MS } from './limits.mjs';

// REQUEST_TIMEOUT_MS is per try: the whole request may take that many times MAX_RETRIES.
export const longestWait = () => REQUEST_TIMEOUT_MS * MAX_RETRIES;
