# Jadis Router (verified against `router/router.ts`, `helpers/router.helper.ts`)

## API
```ts
const routes = defineRoutes({ ... });          // flattens groups, normalizes paths, keeps literal types
const router = new Router(routes, { mode?: 'history' | 'hash', baseUrl?: string });
router.mountOn(el: HTMLElement);               // renders the current URL immediately
router.goto(name, params?);                    // typed: unknown names / missing params are TS errors
router.currentRoute;                           // { name, path, componentSelector }, throws before the first match
router.config;                                 // { mode, baseUrl }
```
Route definition: `{ path: '/x/:id', page: PageClass, options?: { rootComponentSelector?: string } }`. Paths are typed `` `/${string}` ``, so they must start with `/`.

`defineRouteGroup('/prefix', { ... }, { rootComponentSelector? })` prefixes paths. Groups can nest, route names are camel-concatenated (`admin` + `users` → `adminUsers`), and group options merge under each route's own options (the route wins).

Pass `defineRoutes(...)` straight into `new Router(...)`, or keep it in a `const`, so that `goto` keeps literal key and param types.

## How it renders
On every URL change (and on `mountOn`):
1. It finds the **first** route (in definition order) whose regex matches. Each `:param` becomes `(.+)`, which **also matches `/`**, and static segments are not regex-escaped.
2. **No match → it throws** `No route found for path: …` (uncaught, inside the event handler or `mountOn`).
3. It builds a fresh element: `createElement(rootSelector, { attrs: params })` with the page `createElement(pageSelector, { attrs: params })` appended inside it. Without a root, it's just the page.
4. `mountPoint.replaceChildren(newElement)`.

Consequences:
- **Order matters**: declare `/user/:id/edit` before `/user/:id`, and a catch-all last: `notFound: { path: '/:rest', page: NotFoundPage }` (it matches any non-root path; `rest` arrives as an attribute).
- **Every navigation creates new instances** of the page **and** the layout, even when navigating to the same route or between two routes that share a layout. Layout state (an open menu, scroll position, fetched user) is lost. Keep it in a service, or render the persistent shell outside the mount point and mount the router on an inner element.
- Params arrive as **attributes** (on the layout too). Read them with `useAttributes({ id: (v) => … })` or `this.getAttribute('id')`. They're strings and they're not URL-decoded.
- A layout with shadow DOM must render a `<slot>`, or the page will be invisible.
- Page and layout classes must be registered (their modules imported) before navigation. Importing them into `router.ts` takes care of that.
- There are no query-string helpers. Read `location.search` / `URLSearchParams` yourself; `goto` doesn't set them.
- There are no guards, redirects, lazy loading or nested outlets. Implement guards by checking before calling `goto` (or inside the page's `onConnect`, then `goto` elsewhere).

## Modes
- `history` (default): `goto` → `history.pushState(baseUrl + path)`. Back/forward → `popstate`. The server or dev host must fall back to `index.html` (Vite dev does; for static hosting configure SPA fallback, e.g. a GitHub Pages `404.html` copy).
- `hash`: the URL is `/#/path`. `goto` still uses `pushState` (with `#/path`). Back/forward and plain `<a href="#/about">` links work via `hashchange`. This is the simplest choice for static hosting.
- `baseUrl` is stripped from `location.pathname` when matching and prepended on `goto`. Set it to the deploy subdirectory (matching Vite's `base`).

## Links
The router does **not** intercept `<a>` clicks. In history mode a plain `<a href="/about">` triggers a full page reload. Options:
```tsx
<a href="/about" onClick={(e) => { e.preventDefault(); router.goto('about'); }}>About</a>
```
or a small reusable link component that takes a route name. In hash mode, `href="#/about"` works without JS.

## Accessing the router from pages
The scaffold exports `myRouter` from `src/router.ts`, and pages import it. That's a circular import (router → pages → router), and it's safe **only** because pages touch `router` inside handlers at runtime, never at module top level. Keep it that way, or emit a navigation request on a Bus that `router.ts` listens to.

`main.ts` should just `import './router';` (plus global CSS).
