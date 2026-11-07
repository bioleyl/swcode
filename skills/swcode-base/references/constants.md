# Named constants

Details for rule 6.

## Why

A bare `100` or `"project"` tells the reader nothing; `SCORE.nameExactMatch` or `SCOPE.project` says what it means, and changing it happens in one place.

## What needs a constant

- Numbers and strings with domain meaning: limits, durations, scores, status codes, environment variable names, file paths, storage keys, event names, attribute names, selectors.
- Any string used in more than one place.
- A literal that crosses a boundary the compiler can't check (a selector, an attribute name, a storage key): import the constant instead of repeating the literal, in code and in tests alike.

## What stays literal

- A literal whose meaning is plain from the expression: `index + 1`, `items.length === 0`.
- A literal the compiler already checks against a typed API: a member of a union type, a DOM event name in `addEventListener('click', …)`.
- CSS class names: `class="label"`, `ref('.label')`, `classList.add('fading')`, never `<NAME>_CLASS` constants. The `.css` file that styles them repeats them as literals anyway, so a constant only hides the name from someone reading the template next to its styles.
- An `id` used only inside its own template: the `htmlFor`/`id` pair of a label and its field, a `ref('#ticket')` in the same component. An id that other files or tests look up gets a constant.
- Test inputs and expected values: a test reads best when the numbers are in front of you. Attribute names, storage keys and other non-class selectors in tests still come from the constants.

## Where constants live

- A module's fixed values live in a `<name>.constants.ts` file next to it (`button.tsx` → `button.constants.ts`), so the module stays short, and other code and tests import the values without loading the module.
- Values several modules share live in one shared constants file at the root of the source tree: `src/shared.constants.ts`.

## How to name them

- After their module, so they can be re-exported without a clash: `BUTTON_EVENT`, not `EVENT`.
- A lookup is a singular `as const` object keyed by what it names: `BUTTON_ATTRIBUTE.busyLabel`.
- A list is a plural array, with its derived type beside it: `BUTTON_VARIANTS`.
- A single value is a plain constant: `LOGIN_MAX_ATTEMPTS`.

```ts
// button.constants.ts
export const BUTTON_ATTRIBUTE = {
  busyLabel: 'busy-label',
  variant: 'variant',
} as const;

export const BUTTON_VARIANTS = ['primary', 'ghost', 'danger'] as const;
export type ButtonVariant = (typeof BUTTON_VARIANTS)[number];
```

The derived type stays beside the array, not in `types/`: the array is its source of truth (see [types.md](types.md)).

## Values shared between languages

When the same value lives in two languages (a field name or a length limit in the TypeScript form and in the PHP handler), comments that say "keep in step with…" keep nothing in step. In order of preference:

1. One source both read: a JSON file the build imports and the server reads.
2. One source that generates the other at build time.
3. A test that reads both and fails when they differ.

## Bad / Good

```ts
// Bad: login.ts
if (attempts > 3) {
  throw new TooManyAttemptsError();
}
localStorage.setItem('syware.lang', lang);
```

```ts
// Good: login.constants.ts
export const LOGIN_MAX_ATTEMPTS = 3;
export const LOGIN_STORAGE_KEY = {
  lang: 'syware.lang',
} as const;

// Good: login.ts
if (attempts > LOGIN_MAX_ATTEMPTS) {
  throw new TooManyAttemptsError();
}
localStorage.setItem(LOGIN_STORAGE_KEY.lang, lang);
```
