/** The text with its first letter in upper case. */
export const capitalize = (text) => (text === '' ? '' : text[0].toUpperCase() + text.slice(1));
