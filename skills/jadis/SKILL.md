---
name: jadis
description: Build frontends with Jadis (@jadis/core), a minimal library over native Web Components — no virtual DOM, no reactivity, no framework runtime. Use whenever code imports `@jadis/core`, extends `Jadis`, uses `useRefs`/`useChange`/`useEvents`/`useAttributes`/`onBus`/`Router`/`defineRoutes`, sets `jsxImportSource: "@jadis/core"`, or the user asks to create, scaffold, extend, debug, test or review a Jadis component, page, router or app.
---

# Jadis

Jadis (`@jadis/core`, v1.x) is a thin base class over Custom Elements + Shadow DOM. You render **real DOM once**, then update the nodes yourself. There is no re-render, no diffing, no reactivity graph. Never bring in React/Vue/Lit patterns (state → re-render, hooks rules, keys, `setState`), and never add a second component model.

Docs: <https://bioleyl.github.io/jadis/> · Source: <https://github.com/bioleyl/jadis> · Examples: <https://github.com/bioleyl/jadis-examples> (todo-list, minesweeper, cooking).

Everything below was checked against the 1.0 source (`packages/core/src`). Where the docs and the source disagree, this file follows the source and says so.

References (load when needed):
- [references/api.md](references/api.md): every public API, with exact signatures and runtime behavior
- [references/routing.md](references/routing.md): the Router in depth, with its pitfalls
- [references/setup.md](references/setup.md): scaffolding, Vite, tsconfig, no-build/CDN, JS + JSDoc typing, testing

## Component anatomy (canonical TS)

```tsx
import { Jadis } from '@jadis/core';

import styles from './todo-item.css?raw';          // the styles live in todo-item.css, next to the component

export class TodoItem extends Jadis {
  static readonly selector = 'todo-item';           // must contain a hyphen; createSelector('todo-item') validates at runtime

  // Public useChange fields = typed "inputs" a parent can set via JSX props / createElement props
  readonly text = this.useChange('', (value) => {
    this._refs.label.textContent = value;
  }, { immediate: true });

  // Public useEvents = the component's output API
  readonly events = this.useEvents<{ delete: undefined; toggled: boolean }>();

  private readonly _refs = this.useRefs((ref) => ({
    label: ref('span'),                              // tag names infer the element type
    checkbox: ref<HTMLInputElement>('.check'),       // CSS selectors need an explicit type
    remove: ref('button'),
  }));

  templateHtml(): Node {
    return (
      <div class="row">
        <input type="checkbox" class="check" />
        <span />
        <button type="button">Delete</button>
      </div>
    );
  }

  templateCss(): string {
    return styles;
  }

  onConnect(): void {
    this.on(this._refs.remove, 'click', () => this.events.emit('delete'));
    this.on(this._refs.checkbox, 'change', () => this.events.emit('toggled', this._refs.checkbox.checked));
  }
}

TodoItem.register();                                 // at module bottom; importing the module registers it
```

Parent usage: `<TodoItem text={todo.text} />` (class reference) or `<todo-item />` (registered tag). Then in the parent's `onConnect`: `this._refs.item.events.register('delete', () => …)`.

## Lifecycle: the model everything depends on

1. **Constructor**: the open shadow root is attached (unless `static readonly useShadowDom = false`). Field initializers run here (`useRefs`, `useChange`, `useEvents`, `useAttributes`). Nothing is rendered yet, so **refs throw if accessed now**.
2. **`connectedCallback`**, first time only: `templateCss()` → `<style>` + `templateHtml()` are appended to the shadow root (or to the host when there's no shadow DOM). Then, **synchronously**, the `useAttributes` callbacks fire for every attribute currently set (oldValue `null`).
3. **Next task (`setTimeout`)**: queued `useChange` callbacks run (including `immediate`), **then `onConnect()`**. `onConnect` is **not** synchronous with `appendChild`.
4. **`disconnectedCallback`**: `killSignal` aborts, which removes every listener added with `this.on`, `useEvents().register`, `onBus` or `{ signal: this.killSignal }`. Then `onDisconnect()` runs. **The DOM is kept.**
5. **Reconnection** (moving a node counts): a fresh `killSignal`, **no re-render**, attribute callbacks fire again for current values, and `onConnect()` runs again.

Rules that follow:
- Wire **all** listeners in `onConnect()` through `this.on` / `onBus` / `events.register`, never in the constructor. Manual `removeEventListener` is a code smell.
- `onConnect` must be **idempotent**: it can run many times on the same DOM. Guard one-time work (initial list population, fetches) with a check such as `if (list.childElementCount === 0)`.
- Timers, observers, sockets, and `fetch` should use `killSignal` (`fetch(url, { signal: this.killSignal })`) or be stopped in `onDisconnect`.
- After an `await` in `onConnect`, the component may have been removed. Check `this.isConnected` before touching shared state.
- In tests, never assert DOM changes made by `onConnect` right after `appendChild`. Wait a task, or use Playwright's auto-waiting `expect(locator)`.

## Updating the DOM: no re-render, ever

- **Text/attributes**: write to the refs (`ref.textContent = …`, `toggleAttribute`, `classList`).
- **State**: `useChange(initial, (next, prev) => { /* patch DOM */ }, { immediate: true })` → `.get()`, `.set(v)` or `.set(prev => next)`.
  - `set` **always** fires the callback; there's no equality check.
  - Before the first connection (or while disconnected), each callback is **queued** and replayed in order on the next connect.
  - `set` deep-copies the old value with **`structuredClone`**, so the value must be cloneable: plain data only. **No functions, DOM nodes, or class instances that need their prototype**. Storing a function or Node works once, then the **next** `set` throws `DataCloneError`, a confusing delayed failure.
- **Host styling state**: `this.toggleClass('active', cond)` toggles a class on the host; style it with `:host(.active)`.
- **Lists**: build items with JSX and replace them: `this._refs.list.replaceChildren(...items.map((i) => <li data-id={i.id}>{i.name}</li>))`. For long-lived rows that hold their own state, keep a `Map<id, Element>` and append or remove individually. Prefer **event delegation** on the container (`event.target.closest('li[data-id]')`) over a listener per row.
- Don't rebuild the whole template for a local change. `templateHtml()` runs exactly once per instance.

## JSX semantics (source-verified; the docs are imprecise here)

tsconfig: `"jsx": "react-jsx", "jsxImportSource": "@jadis/core"` (no React involved). Never import `jsx`/`jsxs`/`Fragment` by hand. A JSX expression evaluates **immediately to a real DOM `Node`**, so `const el = <TodoItem />` gives you the instance and you can call methods on it.

How each JSX prop is applied, for every element including custom ones:

| Prop | Effect |
|---|---|
| `class` / `className` | `class` attribute (`className` wins if both) |
| key contains `-` (`data-*`, `aria-*`, `my-attr`) | attribute |
| `on<Name>={fn}` | `addEventListener(name.toLowerCase(), fn)`. **Not** tied to `killSignal`, but the nodes live as long as the template, so that's fine. The event is a plain `Event` in typings. |
| `attrs={{ … }}` | attributes (keys kebab-cased) |
| `props={{ … }}` / **any other key** | **JS property** assignment. If the current property value is a `useChange` handler, its `.set(value)` is called instead. |
| `key` | ignored |
| `undefined` values | skipped |

Consequences:
- **`<user-card name="Ada" />` sets the *property* `name`, not the attribute.** A `useAttributes({ name })` callback will **not** fire. To drive attributes from JSX, use `attrs={{ name: 'Ada' }}` (or `data-*`/hyphenated names). The docs' "props are passed as JSX attributes" is wrong in that sense.
- For standard elements, use DOM property names: `htmlFor` (not `for`), `tabIndex`, `className`. Standard tags are type-checked per element; hyphenated tags accept anything.
- `<MyComp foo={x} />` with a class reference is typed from the class's public non-method fields. A public `useChange<T>` field accepts a plain `T`. That's the idiomatic way to pass typed inputs.
- **Register child components before the parent renders.** Importing the child module (which calls `register()`) is enough. Otherwise a property set on a not-yet-upgraded element gets shadowed by the class field when the element upgrades.
- **Typing limits of class references** (verified with tsc): `<MyComp>children</MyComp>` is a type error (`children` collides with `HTMLElement.children`), and so is `attrs={{…}}`. For slotted content or attributes, use the tag form `<my-comp attr="x">…</my-comp>` (open props; the class must be registered, so import its module). `slot="…"` is allowed directly.
- `part="x"` is typed as the `DOMTokenList` property and fails typecheck: use `attrs={{ part: 'x' }}`.
- With `noImplicitOverride`, every Jadis hook (`selector`, `templateHtml`, `onConnect`…) needs `override`; turn the flag off in Jadis packages to keep the usual style.
- Children: nested JSX, `Node`s, arrays (flattened), strings, numbers. `null`, `undefined` and booleans render nothing, so `{cond && <p/>}` works. Fragments `<>…</>` → `DocumentFragment`.
- Without a JSX transform, use `createElement(tag | Class, { props, attrs }, appendTo?)` (see api.md).

## Reading the DOM

- `useRefs((ref) => ({ … }))`: frozen object of **lazy getters**. Each access runs `getElement` again, so cheap, but nothing is cached.
- `this.getElement<T>(selector)` queries the **shadow root** (or the host if there's no shadow DOM). It **throws** when nothing matches. For optional nodes use `this.shadowRoot?.querySelector(...)`.
- `'child-tag >>> button'` pierces a child's shadow root. It works, but treat it as a smell: give the child a public API instead.
- Slotted (light-DOM) children are **not** reachable via `getElement` when shadow DOM is on. Use `this.querySelector` or `slot.assignedElements()`.

## Attributes

```ts
private readonly _attrs = this.useAttributes({
  name: (value, oldValue) => { this._refs.name.textContent = value ?? ''; },
});
// this._attrs.name -> live getAttribute('name') (string | null)
```
- Don't declare `static observedAttributes`. Jadis uses a `MutationObserver`, and the callbacks list is the observation list.
- The callbacks fire synchronously during connection (after render, before `onConnect`), so refs are safe there. Later changes are delivered **asynchronously** (microtask, batched). They re-fire on reconnection. A removed attribute gives `value === null`.
- Since the initial call already populates the DOM, repeating it in `onConnect` is redundant (the docs' examples do it anyway).
- Router params arrive as attributes, so `useAttributes` is the right tool for page components.

## Communication: pick the narrowest

| Direction | Mechanism |
|---|---|
| Parent → child | Call public methods/setters, or set public `useChange` inputs (JSX props, `el.text.set(...)`). |
| Child → parent | `readonly events = this.useEvents<{ name: Payload }>()`. Child `emit`, parent `child.events.register` in the parent's `onConnect`. |
| Unrelated / app-wide | A shared `Bus<{…}>` exported from a `services/*.bus.ts` module; `this.onBus(bus, 'evt', cb)` inside components. |

- `useEvents` dispatches a **non-bubbling, non-composed** `CustomEvent` on the host. Only listeners registered *on that element* receive it, and the callback gets `detail` directly. Payload `undefined` → `emit('x')` with no argument.
- Listeners registered with `child.events.register` are bound to the **child's** `killSignal`. Register them again in the parent's `onConnect` (that's what reconnection requires anyway).
- `bus.register(evt, cb, signal)` outside components **requires** an `AbortSignal`.
- Don't route local parent/child traffic through a global bus.
- Shared app state: plain singleton services (`static getInstance()`) plus a Bus for change notifications, as in the examples. No store library is needed.

## Styling

- **Always put a component's styles in its own `.css` file**, next to it and named after it (`todo-item.tsx` → `todo-item.css`), imported as text: `import styles from './todo-item.css?raw'`. Never write the CSS inline in `templateCss()`; the only exception is a page without a build step (CDN, no bundler), which cannot import a file.
- Fragments several components share (base rules, form controls) go in their own `.css` files too, imported the same way and placed before the component's own: `return css\`${BASE_STYLES}${styles}\`;`. `css\`\`` just concatenates its interpolations; it doesn't escape or scope anything. The result is injected as a single `<style>` in the shadow root.
- What must stay in TypeScript is only what a static file cannot hold: rules generated from data (one per value of a list) or a value the script also uses (a timer length). Pass such a value as a custom property (`:host { --fade: ${FADE_MS}ms; }`) and read it with `var()`. A breakpoint used only in CSS belongs in the `.css` file, as a literal: `@media` and `@container` cannot read `var()`.
- `?raw` is Vite's text import; the types come with `"types": ["vite/client"]`. `import … with { type: 'text' }` does not work in Vite 8. **Vitest empties CSS imports by default, `?raw` included**: set `test: { css: true }` in the Vitest config, or the components run their tests without any styles.
- Global CSS doesn't reach inside. Use `:host`, `:host(.cls)`, `::slotted(sel)`, `::part()`. Theme with CSS custom properties (they inherit through shadow boundaries).
- Custom elements are `display: inline` by default. Set `:host { display: block; }` (or flex/grid) almost always.
- `static readonly useShadowDom = false` → the template is appended to the host. Styles are then **not scoped** and `<slot>` doesn't work. Use it only for deliberate global-CSS or third-party DOM integration.

## Routing (summary; read references/routing.md before non-trivial work)

```ts
// router.ts
import { defineRouteGroup, defineRoutes, Router } from '@jadis/core';
export const router = new Router(defineRoutes({
  home:    { path: '/', page: HomePage },
  user:    { path: '/user/:id', page: UserPage, options: { rootComponentSelector: 'app-layout' } },
  admin:   defineRouteGroup('/admin', { users: { path: '/users', page: AdminUsers } }), // name: adminUsers
  notFound:{ path: '/:rest', page: NotFoundPage },   // LAST: catch-all, otherwise unknown URLs throw
}), { mode: 'history' });
router.mountOn(document.getElementById('app') ?? document.body);
router.goto('user', { id: '42' });                   // names and required params are type-checked
```
Key pitfalls: the **first match wins** and `:param` matches across `/` (put specific routes first). An unmatched URL **throws**. `<a href>` is **not** intercepted (history mode → full reload), so call `router.goto` from click handlers, or use hash mode with `href="#/path"`. Every navigation **recreates** the page *and* its layout, so their state is lost. A layout must contain a `<slot>`.

## Checklist before finishing Jadis code

- [ ] Each component: `static readonly selector` with a hyphen, globally unique (a duplicate selector is **silently ignored** by `register()`), `register()` called once at module bottom, and its module imported before use.
- [ ] `templateHtml()` returns a `Node` (JSX or `createElement`), never an HTML string.
- [ ] Listeners are wired in `onConnect` via `this.on`/`onBus`/`events.register` or `{ signal: this.killSignal }`. Window/document listeners use the signal.
- [ ] `onConnect` is safe to run twice. Timers and observers are cleaned up in `onDisconnect`.
- [ ] `useChange` holds structured-cloneable data only.
- [ ] Attributes you expect `useAttributes` to see are set as attributes (`attrs={{…}}`, `setAttribute`, router params), not JSX props.
- [ ] `:host { display: … }` is set. Shadow DOM stays on unless there's a stated reason.
- [ ] No React-isms: no re-render functions, no keys, no virtual state trees.
- [ ] Type-check (`tsc --noEmit`). Browser behavior (timing, shadow DOM, slots, routing) is tested in Playwright, not jsdom.

## Undocumented exports

`h`, `createVNode`, `createComponentVNode`, `mountVNodes`, `renderToString`, `hydrateSSR` and `VNode` (experimental SSR/VNode helpers) are exported but undocumented. Don't use them unless the user asks. `assert(cond, msg)` (TS `asserts cond`, throws `Error`) is a handy public helper.
