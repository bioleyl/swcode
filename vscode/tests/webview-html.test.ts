import { describe, expect, it } from 'vitest';

import { frameHtml, pageUrl } from '../src/lib/webview-html';

import { OPEN_MESSAGE } from '../src/swcode.constants';

const target = { daemon: 'http://127.0.0.1:4711', token: 'aBc+=' };

describe('the bundle’s page', () => {
  it('asks the daemon for the entry a host ships, with what the host tells it', () => {
    expect(pageUrl(target, 'dark')).toBe(
      'http://127.0.0.1:4711/vscode.html?daemon=http%3A%2F%2F127.0.0.1%3A4711&token=aBc%2B%3D&theme=dark',
    );
  });
});

describe('the webview’s page', () => {
  it('frames the bundle’s page', () => {
    expect(frameHtml(target, 'light')).toContain(
      '<iframe class="swcode-frame" src="http://127.0.0.1:4711/vscode.html?daemon=',
    );
  });

  it('lets the daemon’s origin in, and nothing else', () => {
    const html = frameHtml(target, 'dark');
    expect(html).toContain(
      `content="default-src 'none'; frame-src http://127.0.0.1:4711; style-src 'unsafe-inline';"`,
    );
  });

  it('hands the messages the extension posts over to the frame', () => {
    const html = frameHtml(target, 'dark');
    expect(html).toContain("window.addEventListener('message'");
    expect(html).toContain("frame.contentWindow.postMessage(data, '*')");
  });

  it('sends the bundle’s open request back to the extension, through the vscode api', () => {
    const html = frameHtml(target, 'dark');
    expect(html).toContain('acquireVsCodeApi()');
    // The tag is the one the bundle posts, so the two sides of the channel agree.
    expect(html).toContain(`data.type === '${OPEN_MESSAGE}'`);
    expect(html).toContain('vscode.postMessage(data)');
  });
});
