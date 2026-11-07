# SOLID, without speculation

Reasons and examples for rules 7 and 9–13. Examples declare types inline to stay short; in real code they live in `types/`.

The user designs with SOLID. Each principle names a problem to fix in the code that exists today, not a structure to build in advance. Coding agents mostly get SOLID wrong by over-applying it: interfaces, factories and layers "for flexibility" that nothing uses. Rule 13, **Nothing speculative**, is the counterweight to every rule here.

## Contents

- S: One reason to change (rule 7)
- O: Extend by adding (rule 9)
- L: Subclasses keep the contract (rule 10)
- I: Small interfaces (rule 11)
- D: Rules don't depend on details (rule 12)
- The counterweight: Nothing speculative (rule 13)

## S: One reason to change (rule 7)

**Why:** when one kind of change touches one place, changes stay small and don't break unrelated behaviour.

- Ask who would request a change to this file: the person who owns a business rule, a designer, whoever runs the server. Two different answers mean two concerns: split the file.
- In the frontend, UI and business rules are always two concerns. That's why rules live in services and components only show and report (see [frontend.md](frontend.md)).
- It is not "one function per file" or "the smallest possible classes". A service with ten functions about workflows has one reason to change.

Bad: a `workflows.ts` that decides who may delete a workflow, formats run dates for display, and builds the workflow menu's markup. A new date format, a new menu look and a new deletion rule all land in the same file.

Good: the deletion rule in `services/workflows.ts`, the date format in `lib/format.ts`, the menu in an organism.

## O: Extend by adding (rule 9)

**Why:** code that already works stays untouched when a case is added, and the compiler lists every place the new case must be handled.

The exhaustive mapper (rule 18) is the main tool.

```ts
// Bad: every new channel means another branch, in every function that sends
async function notify(channel: Channel, message: Message): Promise<void> {
  if (channel === 'email') {
    return sendEmail(message);
  }
  if (channel === 'sms') {
    return sendSms(message);
  }
  throw new Error(`Unknown channel: ${channel}`);
}
```

```ts
// Good: a new channel is a new entry, and the Record fails to compile until it has one
const SEND_BY_CHANNEL: Record<Channel, (message: Message) => Promise<void>> = {
  email: sendEmail,
  sms: sendSms,
};

const notify = (channel: Channel, message: Message): Promise<void> => SEND_BY_CHANNEL[channel](message);
```

It is not a plugin system, a hook or an extension point for cases nobody asked for.

Per-case functions are fine when a mapper would lose type checking: if each case has its own set of translation keys, one function per case keeps every key checked, where a `Record` over the cases would widen them to a shared type.

## L: Subclasses keep the contract (rule 10)

**Why:** code written against the parent must keep working with any subclass. A subclass that refuses part of the contract breaks callers that never mention it.

- An override accepts everything the parent accepts, and keeps every promise the parent makes. No extra preconditions, no weaker results, no "not supported" throw, no empty override that silently drops behaviour.
- Components extend the framework's base class. Override its lifecycle methods the way the framework documents, and keep what the base class does.
- Beyond the framework's base class, compose: a component that needs another one's behaviour uses it, it doesn't extend it.

```ts
// Bad
class ReadOnlyStore extends Store {
  override save(item: Item): Promise<void> {
    throw new Error('Not supported');
  }
}
```

```ts
// Good: code that only reads asks for less
interface ReadableStore {
  load(id: string): Promise<Item>;
}

interface Store extends ReadableStore {
  save(item: Item): Promise<void>;
}
```

## I: Small interfaces (rule 11)

**Why:** a function that asks for less is easier to call, easier to test, and doesn't break when unrelated fields change.

- Ask only for what you use. A function that reads a workflow's id and name takes `Pick<Workflow, 'id' | 'name'>`, and a test can pass `{ id, name }` without building a whole workflow.
- Expose only what callers need. A service exports the functions its callers use, not its internal state. A component's public surface is its properties, events and a few methods.
- Split an interface whose users each use a different part of it (see `ReadableStore` above).

```ts
// Bad
function workflowTitle(workflow: Workflow): string {
  return `${workflow.name} (${workflow.version})`;
}

// Good
function workflowTitle(workflow: Pick<Workflow, 'name' | 'version'>): string {
  return `${workflow.name} (${workflow.version})`;
}
```

## D: Rules don't depend on details (rule 12)

**Why:** business rules outlive the details around them (the server's URL, the storage engine, the UI framework), and they can be tested without any of them.

- Business rules receive what they need as arguments: the time, the configuration, the current user.

  ```ts
  // Bad
  function isOverdue(invoice: Invoice): boolean {
    return Date.now() > invoice.dueAt + GRACE_PERIOD_MS;
  }

  // Good
  function isOverdue(invoice: Invoice, now: number): boolean {
    return now > invoice.dueAt + GRACE_PERIOD_MS;
  }
  ```

- Never read from global state what a parameter already carries, or should. A function that takes `locale` and also reads `i18n.locale` can disagree with itself: pass the value in, once.
- When they need the outside world, they go through a gateway the project owns, never through `fetch`, `localStorage` or a library directly.

  ```ts
  // Bad: services/workflows.ts
  await fetch(`/api/workflows/${id}`, { method: 'DELETE' });

  // Good: services/workflows.ts
  await api.workflows.remove(id);
  ```

- Dependencies point toward the rules: components depend on services, services on the gateway, the gateway on `fetch`. Never the other way: a service never imports a component, a rule never imports the gateway's internals.
- A gateway is one module that tests can replace (a module mock, or a fake passed in). It earns an `interface` in `types/` once a test fake implements it too; before that, the interface is speculation.
- An effectful helper performs one operation and returns a structured result. Its caller decides what to do with the results; the helper doesn't mix effects with business rules.

## The counterweight: Nothing speculative (rule 13)

SOLID fixes problems the code has today. Before adding any structure (interface, factory, base class, registry, option, layer), answer three questions:

1. Which change would this structure protect against?
2. Has that change happened, or been asked for?
3. Is there a second implementation or caller today? A test fake counts.

Three noes mean you don't build it. Settings that change per deployment (a sender address, a server path, a key) are configuration, not speculation: the server they run on is their caller. "To make it SOLID" is not an answer to any of them. See mistakes 3 and 4 in [agent-mistakes.md](agent-mistakes.md).
