# Mistakes coding agents tend to make

Read this the first time you load the skill in a session. Each entry is a mistake models make often enough to need its own rule: what it looks like, and what to write instead.

Most of them come from one habit: writing more than the task needs. More guards, more options, more layers, more comments. The fix is restraint, not more knowledge. Before each line, ask whether the task needs it today.

Examples declare types inline to stay short; in real code they live in `types/`.

## Contents

1. Hiding failures
2. Checking for cases the types rule out
3. An abstraction with one implementation
4. Options nobody uses
5. Comments that repeat the code
6. Adapting a near-copy
7. Library calls that don't exist
8. Faking success
9. Vague names
10. One function doing everything
11. Business rules inside a component
12. Leftovers
13. A second way of doing something
14. A package for a few lines
15. Not stopping when the task is done
16. What restraint never cuts

## 1. Hiding failures

Rule: **Don't hide failures.**

```ts
// Bad
async function findEmail(userId: string): Promise<string | undefined> {
  try {
    const user = await users.get(userId);
    return user.email;
  } catch {
    return undefined;
  }
}
```

A database outage now looks the same as "this user has no email", and nobody finds out.

```ts
// Good
async function findEmail(userId: string): Promise<string | undefined> {
  const user = await users.get(userId);
  return user.email;
}
```

Storage errors propagate; `undefined` only ever means "no email". Catch only a failure you can handle, and make it explicit: rethrow it, or return `{ ok: false, error }`.

Failures also hide without any `catch`: in a value that means "it failed" and then gets used as if it were real.

```php
// Bad: if fwrite fails, or a second request reads the file before it is written, the secret is ''
$handle = @fopen($file, 'x');
if ($handle !== false) {
    fwrite($handle, bin2hex(random_bytes(SECRET_BYTES)));
    fclose($handle);
}
return trim((string) file_get_contents($file));
```

Check the result of every call that can fail before using it, and refuse an empty secret: a request without one must fail, not carry on unsigned. In TypeScript the same trap is `?? ''` or `|| []` on a lookup whose `undefined` means it failed.

Another form is recording an outcome before it happens. A message marked as sent before the mail goes out is lost when sending fails: the retry then looks like a duplicate.

## 2. Checking for cases the types rule out

Rule: **Check at the boundary, trust the types inside.**

```ts
// Bad
function orderTotal(items: OrderItem[]): number {
  if (!items || !Array.isArray(items)) {
    return 0;
  }
  return items.reduce((sum, item) => sum + (item?.amountCents ?? 0), 0);
}
```

```ts
// Good
function orderTotal(items: OrderItem[]): number {
  return items.reduce((sum, item) => sum + item.amountCents, 0);
}
```

The type already says `items` is an array of `OrderItem`. The checks can never fire, yet every reader has to wonder whether they can, and a real bug upstream turns into a silent `0`. At the boundary (a request body, a parsed JSON file, `localStorage`) validation is required, not extra.

## 3. An abstraction with one implementation

Rule: **Nothing speculative.**

```ts
// Bad
interface PaymentProcessor {
  charge(amountCents: number): Promise<string>;
}

class CardPaymentProcessor implements PaymentProcessor {
  charge(amountCents: number): Promise<string> {
    return paymentProvider.createCharge(amountCents);
  }
}

function createPaymentProcessor(): PaymentProcessor {
  return new CardPaymentProcessor();
}
```

```ts
// Good
function charge(amountCents: number): Promise<string> {
  return paymentProvider.createCharge(amountCents);
}
```

One implementation means the interface and the factory are ceremony. When a test needs a fake, that's a second implementation: then an interface (in `types/`) earns its place. "Following SOLID" is the usual excuse for this one; SOLID fixes problems the code has, not ones it might have (see [solid.md](solid.md)).

## 4. Options nobody uses

Rule: **Nothing speculative.**

```ts
// Bad: the only caller passes the invoice and nothing else
function renderInvoice(invoice: Invoice, options: RenderOptions = {}): string {
  const { format = 'html', locale = 'fr', debug = false, legacyMode = false } = options;
  // …
}
```

```ts
// Good
function renderInvoice(invoice: Invoice): string {
  // …
}
```

Add the option when its first caller arrives. Watch for names like `enable…`, `use…V2`, `…Mode`: they usually mean a second behaviour nobody asked for.

## 5. Comments that repeat the code

Rule: **Comments say why.**

```ts
// Bad
// Increment the retry counter
retries += 1;

// Step 3: return the result
return invoice;
```

```ts
// Good
// The provider rate-limits per minute, so retries wait for the next window.
await waitForNextMinute();
```

A comment earns its place only when it says something the code can't: a reason, a constraint, a trap.

## 6. Adapting a near-copy

Rules: **Read and reuse**, **Test behaviour and edges.**

Copying a similar function and adjusting it is where off-by-one errors and wrong empty-case handling come from. First search the project for an existing helper and call it. If you write new code, work it out from the requirement, not from the neighbour that looks alike.

```ts
// Bad: adapted from the page-index helper, Math.floor(offset / PAGE_SIZE)
const pageCount = Math.floor(totalItems / PAGE_SIZE);
```

With 45 items and 20 per page this gives 2 pages, and the last 5 items can't be reached.

```ts
// Good
const pageCount = Math.ceil(totalItems / PAGE_SIZE);
```

…with tests for 0, 1, 20, 21 and 45 items.

## 7. Library calls that don't exist

Rule: **Real APIs only.**

Models write calls from memory of what an API usually looks like: a method from a newer version, an option from a similar library, an import from a package that was never installed. Check before calling: the version in the lockfile, the exact signature in the package's type definitions in `node_modules`. A call you couldn't check is a guess; say so.

## 8. Faking success

Rule: **No fake success.**

```ts
// Bad
async function getBalance(accountId: string): Promise<number> {
  return 1000; // TODO: call the billing API
}
```

```ts
// Good
async function getBalance(accountId: string): Promise<number> {
  throw new Error(`getBalance(${accountId}): not implemented, needs the billing API client`);
}
```

The same goes for tests: `it.skip`, deleting an assertion, or loosening `toEqual(expected)` to `toBeDefined()` hides a failure instead of fixing it. Fixture data belongs in tests, never in production code.

## 9. Vague names

Rule: **Names say why.**

```ts
// Bad
function processData(data: Order[]): string[] {
  const result: string[] = [];
  for (const item of data) {
    result.push(item.customer.email);
  }
  return result;
}
```

```ts
// Good
function customerEmails(orders: Order[]): string[] {
  return orders.map((order) => order.customer.email);
}
```

## 10. One function doing everything

Rules: **One job per function**, **Size limits.**

Asked for several things in one breath, models write one long function that reads input, applies the rules, saves and renders. Write one function per job and a short function that calls them in order: see the orchestration example in [functions.md](functions.md).

## 11. Business rules inside a component

Rules: **No rules in components**, **Services hold the rules.**

Asked to "add a delete button for admins", models put the whole thing in the component: the role check, the condition on the workflow, the API call, the error handling. It works, but the rule can't be tested without rendering, and the next screen with a delete button copies it or forgets it.

```tsx
// Bad: organisms/sw-workflow-actions.tsx
const deletable = currentUser()?.role === 'admin' && workflow.runCount === 0;
```

```ts
// Good: services/workflows.ts
export const canDeleteWorkflow = (workflow: Pick<Workflow, 'runCount'>, user: Pick<User, 'role'>): boolean =>
  user.role === 'admin' && workflow.runCount === 0;
```

The component asks the service and shows the answer. See [frontend.md](frontend.md).

## 12. Leftovers

Rule: **No dead code.**

Unused imports, a helper from an approach you dropped, an export kept "for later", a commented-out block. Before delivering, run the linter or search for unused symbols, and remove them.

## 13. A second way of doing something

Rule: **Read and reuse.**

A new `fetch` wrapper when the project has one, a new error class next to an existing set, `snake_case` in a `camelCase` file, `console.log` where the project has a logger. Read the file and a neighbour before writing, and use what's there.

## 14. A package for a few lines

Rule: **No package for a few lines.**

```ts
// Bad
import classNames from 'classnames';

const cardClass = classNames('card', { selected });
```

```ts
// Good
const cardClass = selected ? 'card selected' : 'card';
```

Every dependency is permanent: updates, security advisories, licence checks. A few lines of local code carry none of that.

## 15. Not stopping when the task is done

Rule: [When to stop](../SKILL.md#when-to-stop).

Many agents are tuned to keep going until a task looks completely resolved, and a task with no stated finish line never looks resolved. So they keep going: one more clean-up, a fix for something noticed next door, a new step added to the plan, another self-check pass over code the last pass just changed. Each step looks helpful; together they turn a ten-line fix into a forty-file change nobody asked for, and bury the requested change in it.

Know what "done" means before starting. When it's reached, write the report and stop. What you noticed on the way goes in the report as `flagged:` lines, and next steps as one-line suggestions.

## 16. What restraint never cuts

Restraint stops at the floor in [SKILL.md](../SKILL.md#the-floor-never-cut-these-to-simplify): boundary validation, error handling that prevents data loss, security, and anything the user asked for stay, however much simpler the code would look without them. Removing one of them is a behaviour change, not a clean-up.
