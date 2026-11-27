import type { DictatedText, Dictation, DictationListener, TextAround } from '../types/dictation.types';

const ENDS_IN_SPACE = /\s$/;
// What follows the words said without a space between: a space already, or punctuation that sticks to a word.
const NEEDS_NO_SPACE_BEFORE = /^[\s.,;:!?)]/;

/** What was said so far, from each result's best guess: Chrome starts each after the first with a space, Safari not. */
export const spokenText = (guesses: readonly string[]): string =>
  guesses
    .map((guess) => guess.trim())
    .filter((guess) => guess !== '')
    .join(' ');

/** The field's text with what was said where the caret was, a space apart from the words around it. */
export function textWithSpoken(around: TextAround, spoken: string): DictatedText {
  if (spoken === '') {
    return { caret: around.before.length, text: around.before + around.after };
  }
  const before = around.before === '' || ENDS_IN_SPACE.test(around.before) ? around.before : `${around.before} `;
  const after =
    around.after === '' || NEEDS_NO_SPACE_BEFORE.test(around.after) ? around.after : ` ${around.after}`;
  const said = before + spoken;
  return { caret: said.length, text: said + after };
}

/**
 * A dictation that listens only once the microphone is allowed, where the device asks first (the Android app): until
 * the answer, stopping or aborting ends it without the recognizer ever starting, and a refusal ends it saying
 * `refused`.
 */
export function dictationOnceAllowed(
  allowed: Promise<boolean>,
  recognizer: Dictation & { start(): void },
  listener: Pick<DictationListener, 'ended' | 'failed'>,
  refused: string
): Dictation {
  let state: 'asking' | 'listening' | 'over' = 'asking';
  const endUnheard = (): void => {
    state = 'over';
    listener.ended();
  };
  void allowed.then((isAllowed) => {
    if (state !== 'asking') {
      return;
    }
    if (!isAllowed) {
      listener.failed(refused);
      endUnheard();
      return;
    }
    state = 'listening';
    recognizer.start();
  });
  // Once it listens, the recognizer says its own end; before, there is nothing to stop but the wait.
  const end = (endRecognizer: () => void) => (): void => {
    if (state === 'listening') {
      endRecognizer();
    } else if (state === 'asking') {
      endUnheard();
    }
  };
  return { abort: end(() => recognizer.abort()), stop: end(() => recognizer.stop()) };
}
