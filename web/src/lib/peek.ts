// What starts a question beside the session instead of a message: it never enters the conversation.
const PEEK_LINE = /^\/peek(?:\s+([\s\S]*))?$/;

/** The question of a /peek line: empty for the command alone, nothing for a message. */
export const peekQuestion = (text: string): string | undefined => {
  const line = PEEK_LINE.exec(text.trim());
  return line ? (line[1] ?? '').trim() : undefined;
};
