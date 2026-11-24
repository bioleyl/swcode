import { ColorThemeKind } from 'vscode';

import type { HostTheme } from '../types/webview-html.types';

// The IDE's theme kinds, in the bundle's two: a high-contrast theme reads as its plain one.
// biome-ignore assist/source/useSortedKeys: the order is the order of VS Code's own enum
const THEMES: Record<ColorThemeKind, HostTheme> = {
  [ColorThemeKind.Light]: 'light',
  [ColorThemeKind.Dark]: 'dark',
  [ColorThemeKind.HighContrast]: 'dark',
  [ColorThemeKind.HighContrastLight]: 'light',
};

/** The bundle's theme for the IDE's active one. */
export function themeOf(kind: ColorThemeKind): HostTheme {
  return THEMES[kind];
}
