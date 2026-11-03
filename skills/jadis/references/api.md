# Jadis 1.0 API reference (verified against source)

```ts
import {
  Jadis, Bus, Router, defineRoutes, defineRouteGroup,
  createElement, createSelector, isComponentSelector, css, assert,
  type ChangeStateHandler, type UseEventsHandler, type ChangeOptions,
  type Route, type RouterMode, type RouterOptions,
} from '@jadis/core';
```
JSX runtime entry points: `@jadis/core/jsx-runtime`, `@jadis/core/jsx-dev-runtime`.

## `abstract class Jadis extends HTMLElement`

### Statics
| Member | Notes |
|---|---|
| `static readonly selector: \`${string}-${string}\`` | Required. `register()` throws `selector is not defined for X` if it's missing. |
| `static readonly useShadowDom = true` | `false` → the template is appended to the host itself (no scoping, no slots). |
| `static readonly template = ''` | Reserved, unused. |
| `static readonly observedAttributes = []` | Don't override. Attribute observation goes through `useAttributes`. |
| `static register(): void` | `customElements.define(selector, this)` **if no element is registered under that selector yet**. A second class reusing a selector is silently ignored. |
| `static toTemplate(options?: { props?, attrs? }, slotted?: DocumentFragment): T` | `createElement(selector, options)`, then appends `slotted.cloneNode(true)`. Listeners on the slotted nodes are **not** cloned. |
| `static toString()` | Returns `selector`, so `ref<Child>(Child.toString())` or `` `${Child}` `` work as query strings. |

### Lifecycle hooks (optional overrides)
- `onConnect?(): void`: runs in a `setTimeout` after every connection, after queued `useChange` callbacks. It can be `async`.
- `onDisconnect?(): void`: runs synchronously after `killSignal` aborts.
- `templateHtml?(): Node`: called once, on the first connection.
- `templateCss?(): string`: called once. An empty or missing value means no `<style>` is emitted. The style comes before the HTML.

### Instance members
| Member | Signature / behavior |
|---|---|
| `isConnected` (getter) | Jadis's own flag: `true` between connected and disconnected callbacks. |
| `shadowRoot` | `ShadowRoot \| null` (open mode). |
| `protected killSignal` | `AbortSignal`. Aborted on disconnect, recreated on reconnect. |
| `protected onConnectActions: Array<() => void>` | Queue flushed before `onConnect`. Useful when extending Jadis (see below). |
| `protected attributesCallback` | Internal map filled by `useAttributes`. |
| `protected getElement<E extends HTMLElement = HTMLElement>(query)` | Splits on `>>>` and, for each segment, runs `(el.shadowRoot ?? el).querySelector(seg)`, starting from `this`. Throws `Jadis.getElement: <seg> element is not reachable`. Tag-name queries infer the element type. |
| `protected useRefs(map: (ref) => Record<string, …>): Readonly<…>` | `ref(query)` only records the query. The returned frozen object has getters that call `getElement(query)` on **every** access. `ref<T>(query)` sets the type. |
| `protected useChange<T>(initial, onChange(next, prev), { immediate? }): Readonly<{ get(): T; set(v: T \| ((prev: T) => T)): void }>` | See "useChange details" below. |
| `protected useAttributes<A extends string>(callbacks: Record<A, (value: string \| null, old: string \| null) => void>): Record<A, string \| null>` | Returns enumerable live getters (`getAttribute`). Callbacks: initial sync pass on each connection for non-null attributes (old = `null`), then a batched `MutationObserver` (one call per attribute per batch, with the batch's first oldValue). The observer is disconnected while the element is detached. |
| `protected useEvents<E>()` / `useEvents(schema)` | `Object.freeze({ emit(name, detail?), register(name, cb) })`. `emit` = `this.dispatchEvent(new CustomEvent(name, { detail }))`, non-bubbling and non-composed. `register` adds a listener on `this` with `{ signal: this.killSignal }` and calls `cb(detail)`. |
| `protected on(el: HTMLElement, event: keyof HTMLElementEventMap, cb)` | `addEventListener` with `killSignal`. Typed for standard HTMLElement events only, and the target must be an `HTMLElement` (use `addEventListener(..., { signal: this.killSignal })` for `window`, `document`, or custom event names). `this.on(this, 'click', …)` listens on the host. |
| `protected onBus(bus, event, cb)` | `bus.register(event, cb, this.killSignal)`. |
| `protected toggleClass(name, cond)` | `classList.add`/`remove` on the **host**. |

Note: `useChange`, `useEvents`, `useRefs`, `useAttributes`, `on`, `getElement` and `killSignal` are `protected`. Expose a hook result publicly by assigning it to a public field (`readonly events = this.useEvents<…>()`), not by calling it from outside.

### useChange details
```ts
// source, simplified
const update = (n, o) => isConnected ? onChange(n, o) : onConnectActions.push(() => onChange(n, o));
if (immediate) update(initial, initial);
set(s) { const old = structuredClone(value); value = typeof s === 'function' ? s(value) : s; update(value, old); }
```
- There's no equality check: every `set` fires.
- `structuredClone(old)` is called on every `set`. It throws `DataCloneError` for functions and DOM nodes (on the `set` *after* such a value was stored), and drops class prototypes and getters. Keep the state as plain data. If `T` is itself a function type, `set(fn)` is treated as an updater.
- Updates made while disconnected are **all** queued and replayed in order on the next connection.
- `immediate` queues one call at construction time, which runs on the first connection before `onConnect`.
- `set` with an updater that mutates and returns the same reference still fires, and `prev` is the pre-mutation clone.
- When a parent passes `prop={value}` through JSX/`createElement`/`toTemplate` and the property currently holds a handler with a `set` method, Jadis calls `.set(value)` instead of overwriting it. Typings expose such a field as plain `T`.

## JSX runtime
- `jsx`/`jsxs(type, props)`: `type` is a tag string, a Jadis class (`new type()`), or `Fragment`.
- Prop application order: `attrs` → `class`/`className` → each remaining key (`props` object merged over direct keys): hyphenated key → attribute; `on*` function → `addEventListener(lowercased)`; otherwise a property (with the `useChange` `.set` rule). Reserved keys: `key`, `class`, `className`, `attrs`, `props`, `children`.
- Intrinsic typing: `Partial<writable non-method props of HTMLElementTagNameMap[tag]>` plus `id`, `class`, `title`, `role`, `slot`, `style` (string), `tabIndex`, `data-*`, `aria-*`, and `on${string}` handlers. `${string}-${string}` tags: an open `Record<string, unknown>`.
- `JSX.Element` is `any`. Cast or annotate when you need the concrete type: `const li = <li /> as HTMLLIElement;` or `const item: TodoItem = <TodoItem />;`.

## `createElement`
```ts
createElement<K extends keyof HTMLElementTagNameMap>(tag: K, opts?, appendTo?): HTMLElementTagNameMap[K];
createElement<T extends HTMLElement>(tag: string, opts?, appendTo?): T;
createElement<C extends JadisConstructor>(Class: C, opts?, appendTo?): InstanceType<C>;
// opts: { props?: Partial<non-method fields (useChange<T> → T)>, attrs?: Record<string, string> }
```
It calls `document.createElement(String(tag))` (a class becomes its selector), then applies **props first** (with the `useChange` `.set` rule), then attrs (keys kebab-cased with `toKebabCase`, values `String()`-ed, `undefined` skipped), then `appendTo?.appendChild(el)` (an HTMLElement, ShadowRoot or DocumentFragment).

## `css` tagged template
`css(strings, ...args: (string | number | boolean)[]): string`. Plain concatenation.

## `createSelector(name)` / `isComponentSelector(name)`
Regex `^[^-]+-[^-]+$`: **exactly one hyphen** with non-empty parts. Note that `createSelector('my-big-card')` **throws**, even though `my-big-card` is a valid custom element name. With multiple hyphens, use a plain string literal: `static readonly selector = 'my-big-card'` (the type `${string}-${string}` accepts it).

## `Bus<T>`
```ts
const bus = new Bus<{ saved: { id: string }; reset: undefined }>();   // TS
const bus = new Bus({ saved: Object, reset: undefined });            // JS schema (typing only)
bus.register('saved', (d) => …, signal);   // signal REQUIRED
bus.emit('saved', { id: 'a' });
bus.emit('reset');                          // undefined-payload events: detail optional
```
It's backed by a private `EventTarget`. Events are synchronous and there's no replay of the last value, so a late subscriber misses past events. If you need "current value" semantics, keep the state in a service and use the bus only for change notifications.

## `useEvents` / `Bus` JS schema constructors
`String`, `Number`, `Boolean`, `BigInt`, `Symbol`, `Function`, `Array`, `Object`, any class (→ instance type), or `undefined` (no payload). JSDoc alternative: `/** @type {import('@jadis/core').UseEventsHandler<{ x: string }>} */ events = this.useEvents();`.

## Extending Jadis (official pattern)
```ts
export abstract class AppComponent extends Jadis {
  protected runWhenConnected(fn: () => void): void {
    this.isConnected ? fn() : this.onConnectActions.push(fn);
  }
}
```
Put cross-cutting helpers (loading states, fetch with `killSignal`, i18n) in an app-level abstract base class instead of duplicating them.

## Undocumented / experimental exports
`h` (alias of `jsx` for the classic `jsxFactory`), `Fragment`, `createVNode`, `createComponentVNode`, `mountVNodes`, `renderToString` (VNode → HTML string with declarative shadow DOM `<template shadowrootmode="open">`), `hydrateSSR`, `type VNode`. These are not covered by the docs, so avoid them unless asked.
