/**
 * Calls `type` with each character typed in a hidden input, which stays empty: from keydown on a keyboard, and from
 * the input itself after a dead key (^ on a Swiss keyboard), with AltGr, or on a phone's keyboard, which say little
 * on keydown.
 */
export function watchTyping(input: HTMLInputElement, signal: AbortSignal, type: (char: string) => void): void {
  const flush = (): void => {
    const text = input.value;
    input.value = '';
    for (const char of text) {
      type(char);
    }
  };
  input.addEventListener(
    'keydown',
    (event) => {
      // Shortcuts stay the browser's; AltGr's characters come through the input.
      if (event.isComposing || event.key.length !== 1 || event.ctrlKey || event.metaKey || event.altKey) {
        return;
      }
      event.preventDefault();
      type(event.key);
    },
    { signal }
  );
  input.addEventListener(
    'input',
    (event) => {
      if (event instanceof InputEvent && event.isComposing) {
        return;
      }
      flush();
    },
    { signal }
  );
  input.addEventListener('compositionend', flush, { signal });
}
