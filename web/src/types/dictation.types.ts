/** The field's text on each side of the caret when dictation starts: what is said goes between. */
export interface TextAround {
  before: string;
  after: string;
}

/** The field's text with what was said in it, and where the caret goes: after the words said. */
export interface DictatedText {
  text: string;
  caret: number;
}

/** What a dictation tells the composer: the words heard so far, a failure worth saying, and its end. */
export interface DictationListener {
  heard(spoken: string): void;
  failed(problem: string): void;
  ended(): void;
}

/** A dictation under way: `stop` still takes the words being heard, `abort` drops them and ends at once. */
export interface Dictation {
  stop(): void;
  abort(): void;
}

/**
 * The browser's speech recognition, as much of it as dictation uses: TypeScript's DOM types have its events but not
 * the recognizer, which Chrome, Edge and Safari give under a `webkit` prefix.
 */
export interface SpeechRecognizer {
  continuous: boolean;
  interimResults: boolean;
  lang: string;
  onresult: ((event: SpeechRecognitionEvent) => void) | null;
  onerror: ((event: SpeechRecognitionErrorEvent) => void) | null;
  onend: (() => void) | null;
  start(): void;
  stop(): void;
  abort(): void;
}

export type SpeechRecognizerClass = new () => SpeechRecognizer;

declare global {
  interface Window {
    // biome-ignore lint/style/useNamingConvention: the browser's own name for the recognizer
    SpeechRecognition?: SpeechRecognizerClass;
    webkitSpeechRecognition?: SpeechRecognizerClass;
  }
}
