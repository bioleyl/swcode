---
name: swcode-base
description: The user's coding rules, with a check of your own diff before you deliver. Load before writing, editing, refactoring or reviewing code in any project or language, and keep applying it to every later code change in the session. Covers reading before writing, named constants, SOLID applied without speculative abstraction, frontend business rules kept in services apart from atomic components, guard clauses and exhaustive mappers instead of if/else and switch, function size limits, error handling, TypeScript types in a types/ directory, the mistakes coding agents tend to make, and verification. Run its self-check on your own after writing or changing code, before presenting or committing; don't wait to be asked. Not for conceptual questions, git workflow, CI configuration, prose or data analysis.
---

# Coding style

These are the user's rules for code. Follow them while writing, then check your own diff against them before you show it. Once loaded, they apply to every later code change in the session, not only the first one.

The design comes from SOLID, applied to the code that exists today. The rest exists because coding agents tend to write *more* than the task needs: extra guards, extra options, extra layers, extra comments. When in doubt, ask whether the task needs this line today. If not, leave it out.

## How to use this skill

- **Before writing**: read the rules below. The first time in a session, also read [references/agent-mistakes.md](references/agent-mistakes.md): it shows the mistakes these rules are aimed at.
- **While writing**: apply the rules. For the reasons and examples behind a rule, read the reference it points to.
- **Before delivering**: run the [self-check](#self-check-before-delivery) on your diff, fix what it finds, and end with the report line.
- **After the report**: stop. See [When to stop](#when-to-stop).
- **When asked to review code**: follow [references/review.md](references/review.md). Report findings; don't edit unless asked.
- **In a language other than TypeScript**: apply the intent with [references/other-languages.md](references/other-languages.md).

Code examples in the references declare types inline to stay short; in real code they live in `types/` (rule 25).

## What takes precedence

- The project's own rules win over this skill: its rules file (`CLAUDE.md`, `AGENTS.md`), and the conventions written in its README or CONTRIBUTING.
- Where a codebase consistently follows a different convention (its own constants file name, no `services/` folder), follow the codebase and mention the difference. Never mix two styles in one codebase.
- The project's linter and formatter own what they check (Biome, ESLint, PHPStan…). Run them; this skill covers what they can't judge.
- A framework's own skill or docs decide how to use the framework; this skill decides the shape of the code around it.

## Rules

Each rule has a short name. Use the name, not the number, when you mention a rule in code or in a review.

### A. Before writing

1. **Read and reuse.** Read the file you'll change, at least one neighbour, and the project's rules. Reuse the project's helpers, error types, constants and logging; don't add a second way of doing something the project already does.
2. **Smallest change.** Make the smallest change that solves the problem. Keep unrelated behaviour and the public contract as they are, unless a break is intended and stated.
3. **Real APIs only.** Before calling a library, check that the call exists in the installed version: the version in the lockfile, the signature in the package's type definitions. Never write what the API "should" look like.
4. **No package for a few lines.** Before adding a dependency, check the platform, the standard library and the dependencies already installed. Add one only for real complexity (parsing, dates and time zones, cryptography), never to save ten lines.

### B. Names and constants

5. **Names say why.** A name tells why the thing exists and what it holds or does. `data`, `result`, `item`, `value`, `info`, `temp`, `obj`, `helper`, `manager` and `utils` need a qualifier (`parsedInvoice`, `rawCsv`), and so do `handle…`, `process…` and `do…` when a vague word follows (`processData`, `handleItem`).
6. **Named constants.** A number or string with domain meaning gets a named constant, in a `<name>.constants.ts` file next to its module, or in `src/shared.constants.ts` when several modules share it. Name constants after their module (`BUTTON_EVENT`, not `EVENT`). CSS class names stay plain strings. When two languages need the same value (a field name in the form and in the PHP handler), keep one source both read, or a test that fails when they drift; never comments alone. What may stay literal, and how to name lookups and lists: [references/constants.md](references/constants.md).

### C. Design: SOLID, without speculation

Each principle fixes a problem the code has today; none is a reason to build structure in advance. Reasons and examples: [references/solid.md](references/solid.md).

7. **One reason to change** (S). A module, class or component answers to one concern, so one kind of change touches one place. Business rules and UI are always two concerns.
8. **One job per function.** A function does one thing at one level of detail. Orchestration stays short and reads as phases: setup, validation, execution, rollback, cleanup and rendering each go in a helper. Never change a parameter to pass a result back, and getters and `is…`/`has…` predicates never change state.
9. **Extend by adding** (O). A new case is a new entry in a mapper or a new module, not a new branch in existing functions. When adding one means editing branches in several places, refactor to a mapper first, then add. Separate per-case functions are fine when a mapper would lose type checking each case has (its own set of translation keys, for example).
10. **Subclasses keep the contract** (L). An override accepts everything its parent accepts and keeps every promise its parent makes: no extra conditions, no "not supported" throw, no empty override. Beyond the framework's base class, compose instead of inheriting.
11. **Small interfaces** (I). Ask only for what you use (`Pick<Workflow, 'id' | 'name'>`), and expose only what callers need. Split an interface whose users each use a different part of it.
12. **Rules don't depend on details** (D). Business rules don't touch the network, storage, clock, environment, DOM or components: they take values as arguments, or go through a gateway the project owns. Never read from global state what a parameter already carries, or should (a `locale` parameter beside `i18n.locale`). Dependencies point toward the rules, never back. An effectful helper does one operation and returns a structured result; its caller coordinates.
13. **Nothing speculative.** No interface, factory, base class or registry with a single implementation (a test fake counts as a second), unless it removes a real dependency. No option, flag, environment variable or `…Mode` parameter without a caller today. Settings that change per deployment (a sender address, a server path, a key) belong in configuration and aren't speculative. SOLID alone never justifies an abstraction.

### D. Frontend: services and atomic components

The UI must be able to change without touching a business rule, and the rules must be testable without a DOM. The split is what matters, not the folder names: follow the project's folders. Layers, examples and tests: [references/frontend.md](references/frontend.md).

14. **Services hold the rules.** Business rules (what is allowed, what to compute, what happens on which failure), app state and use cases live in `services/`, with pure calculations in `lib/`. Every server call goes through one gateway module: `services/api.ts` in an app; a site with a single call keeps it in one module. A service never imports a component or reaches into the DOM; app-wide UI it needs (a toast, a confirm dialog, navigation) goes through a function.
15. **Atomic components.** Components come in layers (atoms, molecules, organisms, templates, pages), each built from its own layer or lower ones (and the design system's components), never from a higher one. Atoms and molecules get data through properties or attributes and report through events; the only services they use are presentational ones: translation, theme, formatting.
16. **No rules in components.** Organisms and pages may call services, including the gateway for a plain read or write. As soon as a call needs a decision, that decision moves into a service. The test of the split: any component can be redesigned without touching a service, and any rule can be tested without a DOM.

### E. Control flow and function shape

17. **`if` is for guard clauses.** Use `if` to return or throw early on invalid input, errors and exceptional cases, and never write `else`. Choose a value with a simple ternary, an exhaustive mapper, or a named predicate or function, never with an `if` that assigns. A side effect that depends on a condition is a ternary when there are two branches (`open ? show() : hide()`); a lone side effect, with nothing to do otherwise, may sit in a single `if`. Never nest ternaries: needing more branches means the function does too much, so split it.
18. **Mappers, not `switch`.** Case-based logic uses a typed map, `const icons: Record<OrderStatus, IconName> = {…}`, so adding a case to the type fails to compile until the map handles it.
19. **Size limits.** At most 4 parameters: a fifth means an options object, and values passed in to keep rules free of details (time, configuration, a secret) can travel together as one context object. No flag parameters: a boolean that picks between two behaviours (`render(invoice, true)`) becomes two functions; a boolean that is the value being set (`setBusy(busy)`, `toggle(force)`) is fine. Aim for 20 lines per function and split above 40; the markup of a JSX template doesn't count, the logic around it does. Extract any loop or callback nested more than two levels deep.
20. **Assign once.** Prefer `const`. When a value would be assigned across a `try` or several branches, extract a function that returns it, or returns a `Result`. Don't keep a mutable variable only to carry context between branches (a `currentPath` for error messages): pass the context in, or return it with the result.

Reasons and examples for rule 8 and group E: [references/functions.md](references/functions.md).

### F. Errors and types

21. **Don't hide failures.** A failure stays visible to whoever can act on it. Catch only an error you can handle, and turn it into an explicit failure result or rethrow it. Never let a failure turn into a normal-looking value: a `catch` that returns `undefined` or a default, `?? ''` on a call whose `undefined` means it failed, PHP's `(string)` on a function that returns `false`. Record that something happened only after it did: a message counts as sent once the mail went out, not before. A deliberate fallback is fine when the function's doc comment says so and nothing is lost.
22. **Check at the boundary, trust the types inside.** Data from outside (requests, parsed JSON, storage, environment, other processes) is validated where it enters. Inside, don't add null checks or `typeof` checks for cases the types already rule out.
23. **No `any`, no casts on outside data.** Use `unknown` only at boundaries and narrow it with a type guard or validator. Never `as Foo` on outside data; `as const` and `satisfies` are fine.
24. **States as unions.** Mutually exclusive states are a discriminated union, not optional fields that can contradict each other. Use `interface` for object shapes and `type` for unions and intersections; `undefined` for absent values, not `null`, unless an outside contract requires `null`.
25. **Types in `types/`.** Code files hold only code. Every named `interface` and `type` goes in a flat `types/` directory, in a file named after its code file (`form.service.ts` → `types/form.service.types.ts`). Zod schemas, and types derived from a runtime value, stay beside that value. A short anonymous type used once inline (in `satisfies` or a generic argument) may stay; name it in `types/` once it's reused or grows past a line. Which `types/` directory and how to name shared types: [references/types.md](references/types.md).

### G. Honesty and clean-up

26. **No fake success.** Production code never returns hardcoded results, fixture data, or a stub presented as done. If you can't implement something, throw an explicit "not implemented" error and say what's missing. Never skip, delete or loosen a test to make it pass; if a test looks wrong, say so.
27. **No dead code.** Before delivering, remove unused imports, exports, functions and variables, unreachable branches, and commented-out code. Version control keeps the history.
28. **Comments say why.** Write a comment only for what the code can't show: a reason, a constraint, a trap. A short doc comment on an export that says what it means or promises, beyond its signature, is welcome. Delete comments that repeat the code, and step scaffolding ("Step 2: …").

### H. Refactoring and verification

29. **Refactors keep behaviour.** A refactor keeps the same outputs, errors, side effects and order. A bug found while refactoring is reported and fixed as a separate change, never bundled in.
30. **Test behaviour and edges.** Add or update tests for behaviour and failure paths. Business rules get tests without a DOM: `lib/` functions directly, services with a fake gateway. For ranges and counts, list the edge cases (empty, one, many, exactly at the limit, just past it) and give each one a test, not a comment.
31. **Verify narrowest first.** Run the tests closest to the change, then the whole suite, then type checking and linting across the workspace, then static analysis (Sonar or similar) when the project has it. Fix new issues on the changed code; never silence one without a stated reason. Format before finishing. If you couldn't run a step, or a language in the change has no linter, formatter or type checker in the project, say so.

## The floor: never cut these to simplify

Rules 2, 13, 22 and 27 remove code. These stay, whatever a simplification suggests. Removing one changes behaviour, so keep it or ask. In a review, a removed or missing floor item is filed as `Floor: <name>`.

- **Boundary checks**: validation and escaping at every boundary.
- **No data loss**: error handling that keeps data from being lost.
- **Security**: authorization checks, escaped output, parameterized queries, secrets kept out of code, logs and deploy packages.
- **Requested behaviour**: what the user asked for.

## Breaking a rule on purpose

When a rule is wrong for a specific case, keep the code and add a comment that names the rule, the reason, and when to look at it again:

```ts
// style exception: size limits — mirrors the payment API's 6 required fields; revisit if we wrap that API.
```

On a later pass, a comment like this means the case is settled: don't flag it again. A `style exception` comment with no "revisit" condition is itself a finding.

## Self-check before delivery

Walk your diff once before you show it. Fix every "no", or say why it stays, then re-check only the lines you changed. Two passes at most: whatever is left after that goes in the report as flagged.

1. Did you read the file, a neighbour and the project's rules, reuse what exists, and does the new code match?
2. Is every library call checked against the installed version, and every new dependency justified?
3. Is every literal with domain meaning a named constant, in the right constants file, with one source when two languages share it?
4. Does each new or changed module have one reason to change, ask only for what it uses, and keep its parent's contract?
5. Do business rules stay free of the network, storage, clock, DOM, components and global state, with dependencies pointing toward them?
6. In frontend code: is every business rule in a service or `lib/`, do atoms and molecules take data in and send events out, and does no component use a higher layer?
7. Is every `if` a guard clause or a lone side effect, with no `else`, no `switch`, no nested ternary, and new cases added as mapper entries?
8. Does every new function have at most 4 parameters, no boolean that picks a behaviour, about 20 lines (never over 40), one job, and a name that says why?
9. Does every failure stay visible, with no `catch` or fallback value that hides it, and is success recorded only after it happened? Is outside data validated where it enters, with no checks inside for cases the types rule out?
10. Does every new abstraction, option or flag have a user today? Is any dead code left?
11. Is there no `any`, no cast on outside data, no `null` without an outside contract, and is every named type in `types/`?
12. Is there no hardcoded success or fixture data in production code, and no test skipped or loosened?
13. Does every new comment say why, or what an export means?
14. For a refactor: is the behaviour unchanged?
15. Did you run the checks in order (nearest tests, suite, type check and lint, static analysis, format), and say which ones you couldn't run?

Then report the pass, so the user can see it ran. List each fix as `<file>:<line> — <what changed>` (leave out the line if it isn't stable), and each thing you noticed but didn't change as `flagged: <file> — <what and why>`. End with one line:

```
coding-style: 3 fixed, 1 flagged
```

or `coding-style: clean` when nothing came up. Count only what you actually did; never give a quality score or a percentage. Flagged means not fixed: anything outside the request is flagged, never changed.

## When to stop

The task is done when the request is met, the checks have run and the report is written. Then stop: the report is the last thing you do.

- Before starting, know what "done" means for this request. If it isn't clear, ask.
- Something you notice outside the request (a bug next door, an old pattern, a missing test elsewhere) goes in the report as a `flagged:` line. Don't fix it.
- Suggest next steps in one line each; never start them unasked.
- Don't add steps to your plan because you found more to improve. A new step is only for something the request needs.
- In review mode, the review is the deliverable: write it and stop. Fixing starts only when asked.
