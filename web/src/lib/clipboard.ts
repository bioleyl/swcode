const COPY_COMMAND = 'copy';

/**
 * Copies through a field put in the page for a moment: what a browser still takes where it gives no clipboard, on a
 * page that came over plain HTTP from another computer.
 */
function copyThroughField(text: string): boolean {
  const field = document.createElement('textarea');
  field.value = text;
  field.setAttribute('readonly', '');
  field.style.position = 'fixed';
  field.style.opacity = '0';
  document.body.append(field);
  field.select();
  try {
    return document.execCommand(COPY_COMMAND);
  } catch {
    return false;
  } finally {
    field.remove();
  }
}

/** Copies a text to the clipboard: whether it got there. */
export async function copyText(text: string): Promise<boolean> {
  // No clipboard outside a secure context: the page was opened over plain HTTP.
  const clipboard = navigator.clipboard as Clipboard | undefined;
  if (!clipboard) {
    return copyThroughField(text);
  }
  try {
    await clipboard.writeText(text);
    return true;
  } catch {
    return copyThroughField(text);
  }
}
