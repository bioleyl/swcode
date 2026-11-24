// @vitest-environment jsdom
import { readFileSync } from 'node:fs';

import { describe, expect, it } from 'vitest';

import { hostConfig } from '../src/services/host';
import { applyTheme } from '../src/services/theme';

// The sheet the host's page loads, read as text: the test checks which colour each token takes, not how it paints.
// Vitest runs from the repository's folder, which is where the sheet lives.
const CSS_SHEET = 'web/src/vscode.css';
const css = readFileSync(CSS_SHEET, 'utf8');

const THEME_ATTRIBUTE = 'data-theme';

// The host's colour each of the app's tokens takes, and its own colour as the fallback.
const HOST_COLOURS: Record<string, string> = {
  '--jl-accent': '--vscode-button-background',
  '--jl-bg': '--vscode-sideBar-background',
  '--jl-code-bg': '--vscode-textCodeBlock-background',
  '--jl-font-mono': '--vscode-editor-font-family',
  '--jl-font-sans': '--vscode-font-family',
  '--jl-line': '--vscode-panel-border',
  '--jl-line-strong': '--vscode-panel-border',
  '--jl-muted': '--vscode-descriptionForeground',
  '--jl-on-accent': '--vscode-button-foreground',
  '--jl-surface': '--vscode-sideBar-background',
  '--jl-surface-2': '--vscode-sideBarSectionHeader-background',
  '--jl-surface-3': '--vscode-input-background',
  '--jl-text': '--vscode-foreground',
  '--jl-text-2': '--vscode-foreground',
};

describe('the theme the host names, from the address to the page', () => {
  it('puts the theme the address carries on the page', () => {
    applyTheme(hostConfig('?theme=dark&token=9f2c').theme);
    expect(document.documentElement.getAttribute(THEME_ATTRIBUTE)).toBe('dark');

    applyTheme(hostConfig('?theme=light').theme);
    expect(document.documentElement.getAttribute(THEME_ATTRIBUTE)).toBe('light');
  });

  it('leaves the page the system’s theme when the address names none', () => {
    document.documentElement.setAttribute(THEME_ATTRIBUTE, 'dark');
    applyTheme(hostConfig('?token=9f2c').theme);
    expect(document.documentElement.hasAttribute(THEME_ATTRIBUTE)).toBe(false);
  });
});

describe('the host’s colours over the app’s tokens', () => {
  it.each(Object.entries(HOST_COLOURS))(
    '%s takes the host’s %s, keeping its own as the fallback',
    (token, host) => {
      expect(css).toContain(`${token}: var(${host}, var(${token}))`);
    }
  );
});
