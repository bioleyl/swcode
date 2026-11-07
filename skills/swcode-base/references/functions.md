# Functions and control flow

Reasons and examples for rule 8 and rules 17–20. Examples declare types inline to stay short; in real code they live in `types/`.

## Contents

- One job per function (rule 8)
- `if` is for guard clauses (rule 17)
- Mappers, not `switch` (rule 18)
- Size limits (rule 19)
- Assign once (rule 20)

## One job per function (rule 8)

**Why:** a function that does one thing can be named, tested and replaced on its own.

Orchestration reads as a list of phases, each a helper with its own name:

```ts
async function submitContact(form: HTMLFormElement): Promise<void> {
  const input = readContactForm(form);
  const validation = validateContact(input);
  if (!validation.ok) {
    showErrors(form, validation.errors);
    return;
  }
  const sent = await postContact(validation.value);
  showOutcome(form, sent);
}
```

Return results instead of changing a parameter:

```ts
// Bad
function addTotals(order: Order, summary: Summary): void {
  summary.totalCents = order.items.reduce((sum, item) => sum + item.amountCents, 0);
}
```

```ts
// Good
function orderTotal(order: Order): number {
  return order.items.reduce((sum, item) => sum + item.amountCents, 0);
}
```

Getters and predicates (`is…`, `has…`, `can…`) only answer; they never change state.

## `if` is for guard clauses (rule 17)

**Why:** when every `if` returns or throws, the main path reads straight down, with no branch to keep in your head.

A chain of guard clauses is fine. Never write `else`, and never choose a value with an `if`: use a ternary, a mapper, or a named function. Building a value by mutation (`menu.push` under a condition) is choosing a value too.

```ts
// Bad
let feeCents = baseFeeCents;
if (order.express) {
  feeCents = baseFeeCents * EXPRESS_FEE_FACTOR;
}
```

```ts
// Good
const feeCents = order.express ? baseFeeCents * EXPRESS_FEE_FACTOR : baseFeeCents;
```

```ts
// Bad
const menu = [...BASE_MENU];
if (user.isAdmin) {
  menu.push(ADMIN_MENU_ENTRY);
}
```

```ts
// Good
const menu = user.isAdmin ? [...BASE_MENU, ADMIN_MENU_ENTRY] : BASE_MENU;
```

A side effect that depends on a condition is a ternary when there are two branches:

```ts
// Bad
if (expanded) {
  menu.open();
} else {
  menu.close();
}
```

```ts
// Good
expanded ? menu.open() : menu.close();
```

Better still, when the API takes the condition itself: `link.classList.toggle('current', current)`.

A lone side effect, with nothing to do otherwise, may sit in a single `if`, since no ternary fits:

```ts
if (sessionJustEnded()) {
  form.showNotice(t('login.sessionEnded'));
}
```

A nested ternary means the logic needs its own function, written as guard clauses:

```ts
// Bad
const size = width > BREAKPOINT.large ? 'large' : width > BREAKPOINT.medium ? 'medium' : 'small';
```

```ts
// Good
function sizeForWidth(width: number): Size {
  if (width > BREAKPOINT.large) {
    return 'large';
  }
  if (width > BREAKPOINT.medium) {
    return 'medium';
  }
  return 'small';
}
```

`'large'`, `'medium'` and `'small'` can stay literal: they are members of the `Size` union, so the compiler checks them.

## Mappers, not `switch` (rule 18)

**Why:** a `Record` keyed by a union must list every member, so a new case fails to compile until it is handled. A `switch` with a `default` silently swallows it. This is also how code stays open to new cases without editing old branches (rule 9).

```ts
// Bad
function statusIcon(status: OrderStatus): IconName {
  switch (status) {
    case 'pending':
      return 'clock';
    case 'paid':
      return 'check';
    default:
      return 'question';
  }
}
```

Adding `'refunded'` to `OrderStatus` falls into `default` without a word.

```ts
// Good: in order.constants.ts
export const ORDER_STATUS_ICON: Record<OrderStatus, IconName> = {
  pending: 'clock',
  paid: 'check',
  cancelled: 'cross',
};

// at the call site
const icon = ORDER_STATUS_ICON[order.status];
```

The values can be functions when each case needs its own behaviour:

```ts
const RENDER_CHECKOUT_STEP: Record<CheckoutStep, () => HTMLElement> = {
  cart: renderCart,
  address: renderAddress,
  payment: renderPayment,
};
```

## Size limits (rule 19)

**Why:** limits you can count are limits you can check. Past them, a function almost always does more than one job.

- **Parameters: at most 4.** A fifth means an options object.

  ```ts
  // Bad
  function sendMail(to: string, subject: string, body: string, replyTo: string, attachments: File[]): Promise<void>

  // Good
  function sendMail(mail: OutgoingMail): Promise<void>
  ```

- **Context objects.** Rule 12 has you pass the time, the configuration or a secret in, instead of reading them. When that pushes a function past 4 parameters, group them: `decide(request, state, context)`, with `context` holding `now`, `config` and `secret`.
- **No flag parameters.** A boolean that picks between two behaviours says nothing at the call site: `render(invoice, true)`. Write `renderHtml(invoice)` and `renderPdf(invoice)`. A boolean that is the value being set is fine: `setBusy(busy)`, `toggle(force)`, `markCurrent(link, current)`.
- **Lines: aim for 20, split above 40.** The markup of a JSX template doesn't count; the logic around it does.
- **Nesting: at most two levels.** A loop or callback inside a loop or callback is the limit; extract the inner operation into a named function.

A case that truly needs more takes a `style exception` comment (see SKILL.md).

## Assign once (rule 20)

**Why:** a `const` has one meaning for its whole life. A `let` assigned in several places makes the reader trace every path to know what it holds.

A value assigned across a `try` becomes a function that returns it:

```ts
// Bad
let config: Config | undefined;
try {
  config = parseConfig(await readFile(CONFIG_PATH, 'utf8'));
} catch {
  config = undefined;
}
```

```ts
// Good
type Result<T> = { ok: true; value: T } | { ok: false; error: unknown };

async function loadConfig(path: string): Promise<Result<Config>> {
  try {
    return { ok: true, value: parseConfig(await readFile(path, 'utf8')) };
  } catch (error) {
    return { ok: false, error };
  }
}

const loaded = await loadConfig(CONFIG_PATH);
```

Use the project's own `Result` type if it has one.

A variable that only carries context for an error message becomes part of the result:

```ts
// Bad
let currentPath = '';
try {
  for (const path of paths) {
    currentPath = path;
    await copyFile(path, target);
  }
} catch (error) {
  logger.error(`Copy failed: ${currentPath}`, error);
}
```

```ts
// Good
async function copyOne(path: string, target: string): Promise<CopyOutcome> {
  try {
    await copyFile(path, target);
    return { ok: true, path };
  } catch (error) {
    return { ok: false, path, error };
  }
}
```

The caller gets the path back with every failure and decides whether to stop or go on.
