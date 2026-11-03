# Setting up a Jadis project

## Scaffold (preferred)
```bash
npx @jadis/create ts my-app        # TypeScript + Vite + JSX + Router demo
npx @jadis/create js my-app        # JS (+ JSDoc checking) + Vite + JSX
npx @jadis/create vanilla my-app   # no bundler, no JSX, import map from a CDN
cd my-app && npm install && npm run dev
```
Scaffold layout (TS):
```
index.html                 <div id="app"></div> + <script type="module" src="/src/main.ts">
src/main.ts                import './router';
src/router.ts              defineRoutes + export const myRouter = new Router(...); mountOn(#app)
src/pages/<name>/<Name>Page.tsx (+ <Name>Page.css next to it, imported with ?raw)
src/components/<Name>.tsx
src/vite-env.d.ts
```
The examples repo uses the same structure plus `services/` (singletons, `*.bus.ts`) and `types/`. File names there are kebab-case (`todo-item.tsx`).

## Manual (Vite + TS)
```bash
npm create vite@latest my-app -- --template vanilla-ts
npm i @jadis/core
```
tsconfig `compilerOptions` (scaffold values):
```json
{
  "jsx": "react-jsx",
  "jsxImportSource": "@jadis/core",
  "target": "ES2020", "module": "ESNext", "moduleResolution": "bundler",
  "lib": ["ES2020", "DOM", "DOM.Iterable"],
  "strict": true, "useDefineForClassFields": true, "isolatedModules": true,
  "noEmit": true, "skipLibCheck": true
}
```
Files that contain JSX must be `.tsx`/`.jsx`. Build with `tsc && vite build`.

## JavaScript with JSX
Put this in `jsconfig.json` (same `jsx`/`jsxImportSource`), or use per-file pragmas:
```js
/// <reference types="@jadis/core/jsx-runtime" />
/** @jsxImportSource @jadis/core */
// @ts-check   (optional: enables JSDoc type checking)
```
JS conventions: `static selector = createSelector('x-y')` (no `readonly`), `useEvents({ evt: String })`, `new Bus({ evt: Number })`, `/** @type {HTMLInputElement} */` before `ref(...)` entries, and `/** @type {import('@jadis/core').UseEventsHandler<{...}>} */`.

## No build step (CDN + import map)
Browsers run neither JSX nor TS, so use `createElement`:
```html
<script type="importmap">
  { "imports": { "@jadis/core": "https://esm.sh/@jadis/core@1.3.0" } }
</script>
<script type="module" src="./src/main.js"></script>
```
```js
import { createElement, Jadis } from '@jadis/core';
class HelloWorld extends Jadis {
  static selector = 'hello-world';
  templateHtml() {
    const frag = document.createDocumentFragment();
    createElement('h1', { props: { textContent: 'Hello' } }, frag);
    return frag;
  }
}
HelloWorld.register();
```
For precompiled TSX served without a bundler (`tsc --jsx react-jsx --jsxImportSource @jadis/core`), also map `"@jadis/core/jsx-runtime": "https://esm.sh/@jadis/core@1.0.0/jsx-runtime"`. Serve over HTTP, because `file://` breaks modules. Pin the version.

## Testing
- **Vitest + jsdom**: pure services, helpers, and simple components. jsdom has custom elements and shadow DOM, but timing (`setTimeout` before `onConnect`, `MutationObserver` delivery) needs awaiting: `await new Promise((r) => setTimeout(r));` after `appendChild`.
- **Playwright**: anything about lifecycle, reconnection, slots, focus/keyboard, routing (history/hash), or CSS. Use auto-waiting assertions (`await expect(page.locator('my-comp >> text=…')).toBeVisible()`). Playwright CSS locators pierce open shadow roots automatically.
- Test the reconnection path: remove the node, re-append it, and assert that listeners still work exactly once (no duplicates, none missing).
