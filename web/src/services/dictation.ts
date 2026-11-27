import { dictationOnceAllowed, spokenText } from '../lib/dictation';
import { inAndroidApp } from './native-app';
import { microphoneAllowed } from './native-microphone';

import type { Dictation, DictationListener, SpeechRecognizerClass } from '../types/dictation.types';

const NOT_ALLOWED_ON_PAGE = 'The microphone is not allowed: allow it in the browser’s settings for this site.';
// The app has no site to allow: the permission is Android's, for swcode.
const NOT_ALLOWED_IN_APP = 'The microphone is not allowed: allow it for swcode in Android’s settings.';

/** What each failure tells the person; nothing when the dictation was ended on purpose. */
const PROBLEMS: Readonly<Record<SpeechRecognitionErrorCode, string | undefined>> = {
  aborted: undefined,
  'audio-capture': 'No microphone was found.',
  'language-not-supported': 'Speech recognition does not know your browser’s language.',
  network: 'Speech recognition could not reach its service: this browser may not offer it.',
  'no-speech': 'Nothing was heard: speak once the microphone is red.',
  'not-allowed': NOT_ALLOWED_ON_PAGE,
  'phrases-not-supported': 'Speech recognition refused the request.',
  'service-not-allowed': 'Speech recognition is not allowed in this browser.',
};

const isKnownCode = (code: string): code is SpeechRecognitionErrorCode => Object.hasOwn(PROBLEMS, code);

/** A failure in words, a code newer than this page's list said as it is. */
function problemOf(code: string): string | undefined {
  if (!isKnownCode(code)) {
    return `Speech recognition failed (${code}).`;
  }
  return code === 'not-allowed' && inAndroidApp() ? NOT_ALLOWED_IN_APP : PROBLEMS[code];
}

/**
 * The browser's recognizer: Chrome, Edge and Safari have one, Firefox none. Outside a secure context (a phone on
 * the network's plain http) the microphone is refused anyway, so there is none either.
 */
const recognizerClass = (): SpeechRecognizerClass | undefined =>
  window.isSecureContext ? (window.SpeechRecognition ?? window.webkitSpeechRecognition) : undefined;

/** Whether the person can dictate here. */
export const canDictate = (): boolean => recognizerClass() !== undefined;

const guessesOf = (results: SpeechRecognitionResultList): string[] =>
  Array.from(results, (result) => result[0]?.transcript ?? '');

/**
 * Listens until stopped, or until the browser ends it after a silence: the words heard so far come to `heard` each
 * time they change, the last guesses still changing, and a failure comes in words to `failed`.
 */
export function dictate(listener: DictationListener): Dictation {
  const Recognizer = recognizerClass();
  if (Recognizer === undefined) {
    throw new Error('This browser has no speech recognition: the microphone should not show.');
  }
  const recognizer = new Recognizer();
  recognizer.continuous = true;
  recognizer.interimResults = true;
  // The page says it is in English; the person speaks the language they chose for their browser.
  recognizer.lang = navigator.language;
  recognizer.onresult = (event) => listener.heard(spokenText(guessesOf(event.results)));
  recognizer.onerror = (event) => {
    const problem = problemOf(event.error);
    if (problem !== undefined) {
      listener.failed(problem);
    }
  };
  recognizer.onend = () => listener.ended();
  if (inAndroidApp()) {
    // The WebView's recognizer never asks for the microphone: Android is asked first, and it listens after a yes.
    return dictationOnceAllowed(microphoneAllowed(), recognizer, listener, NOT_ALLOWED_IN_APP);
  }
  recognizer.start();
  return { abort: () => recognizer.abort(), stop: () => recognizer.stop() };
}
