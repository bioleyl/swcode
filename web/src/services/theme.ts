const STORAGE_KEY = 'swcode.theme';
const THEME_ATTRIBUTE = 'data-theme';
const DARK_QUERY = '(prefers-color-scheme: dark)';
const THEMES = ['light', 'dark'] as const;
export type Theme = (typeof THEMES)[number];

const isTheme = (value: string | null): value is Theme => THEMES.some((theme) => theme === value);

const storedTheme = (): Theme | undefined => {
  try {
    const value = localStorage.getItem(STORAGE_KEY);
    return isTheme(value) ? value : undefined;
  } catch {
    return undefined;
  }
};

/** The theme on screen: the one people picked, or the system's. */
export const currentTheme = (): Theme =>
  storedTheme() ?? (window.matchMedia(DARK_QUERY).matches ? 'dark' : 'light');

/** Puts the picked theme on the page; with none picked, the tokens follow the system. */
export function applyTheme(): void {
  const theme = storedTheme();
  if (theme === undefined) {
    document.documentElement.removeAttribute(THEME_ATTRIBUTE);
    return;
  }
  document.documentElement.setAttribute(THEME_ATTRIBUTE, theme);
}

/** Switches to the other theme and keeps the choice for the next visit. */
export function toggleTheme(): Theme {
  const next: Theme = currentTheme() === 'dark' ? 'light' : 'dark';
  try {
    localStorage.setItem(STORAGE_KEY, next);
  } catch {
    // Without storage, the choice lasts as long as the page.
  }
  document.documentElement.setAttribute(THEME_ATTRIBUTE, next);
  return next;
}
