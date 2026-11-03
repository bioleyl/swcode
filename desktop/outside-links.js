// In swcode's window: links to other sites open in the person's browser, sent to the app, which opens them
// (protocol.DesktopRequest's openOutside).
(() => {
  const outside = (href) => {
    try {
      const url = new URL(href, location.href);
      return /^https?:$/.test(url.protocol) && url.origin !== location.origin ? url.href : undefined;
    } catch {
      return undefined;
    }
  };
  // Links inside the components' shadow roots too: the click's path goes through them.
  addEventListener(
    'click',
    (event) => {
      const link = event.composedPath().find((node) => node instanceof HTMLAnchorElement && node.href);
      const url = link && outside(link.href);
      if (!url) {
        return;
      }
      event.preventDefault();
      window.chrome.webview.postMessage(JSON.stringify({ type: 'openOutside', url }));
    },
    true
  );
  const open = window.open.bind(window);
  window.open = (href, ...rest) => {
    const url = href === undefined ? undefined : outside(String(href));
    if (!url) {
      return open(href, ...rest);
    }
    window.chrome.webview.postMessage(JSON.stringify({ type: 'openOutside', url }));
    return null;
  };
})();
