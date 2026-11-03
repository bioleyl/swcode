---
name: coding-style
description: The user's personal coding style. Load before writing, refactoring, or reviewing code in any language or project — covers magic numbers/strings, separation of I/O from domain logic, guard clauses, exhaustive mappers instead of switch, no nested ternaries, function shape, types in a separate directory, and verification before finishing.
---

# Coding style

Understand the existing architecture and conventions before changing code. Make the smallest change that solves the problem and preserve unrelated behavior.

## No magic numbers

Always use named `const` values for literal numbers that carry domain meaning. A bare `100` or `60` tells the reader nothing; `SCORE.nameExactMatch` or `MAX_RETRY_COUNT` communicates intent and makes future adjustments localized.

## No magic strings

Always use named `const` values for literal strings that carry domain meaning. A bare `"project"` or `"user"` tells the reader nothing; `SCOPE.project` or `LOCKFILE_PATH` communicates intent. This applies to status codes, environment variable names, file paths, and any string used in more than one place.

## Structure

- Keep responsibilities separated: domain rules should not perform I/O, and orchestration should not contain low-level implementation details.
- Prefer small functions with one clear responsibility and descriptive names.
- Keep pure logic deterministic and move filesystem, network, time, environment, and process effects to explicit boundaries.
- Prefer simple composition over premature abstractions. Introduce an abstraction when it removes a real dependency or clarifies a stable boundary.

## Control flow

- Use guard clauses and early returns for invalid input, errors, and exceptional cases.
- Use `if` only as a guard clause that returns (or throws) early. For any other branching, use a simple ternary, an exhaustive mapper, or a named predicate or function.
- Prefer exhaustive mappers over `switch` statements for type- or case-based logic. Use a typed map such as `const labels: Record<MyType, string> = { ... }`; adding a member to `MyType` then produces a compile-time error until the mapper is updated.
- Never use nested ternaries. If the logic needs more branching than a simple ternary or an early return, the function is doing too much: split it into smaller functions with clear responsibilities.
- Avoid deeply nested loops and callbacks; extract the inner operation when it needs its own explanation.

## Function shape

- Prefer `const` over `let`. If a value would need to be assigned across a `try` block or multiple branches, extract that work into a named function that returns a `Result` or an explicit value.
- Keep orchestration functions short and phase-oriented. Extract setup, validation, execution, rollback, cleanup, and rendering into separate helpers when they have distinct responsibilities.
- Keep effectful boundaries explicit. A filesystem or network helper should perform one operation and return a structured result; callers should coordinate those results rather than mixing low-level effects with business rules.
- Avoid mutable state used only to carry context between branches, such as a `currentPath` variable for error reporting. Pass the context into a helper or return it with the operation result.
- Prefer named predicates and helpers over inline conditionals when they make a failure path or invariant easier to understand.

## Type safety (TypeScript)

- Never use `any`. Use `unknown` only at system boundaries (parsed JSON, network, environment, `catch`), and narrow it with a type guard or validator before use.
- Never cast external data with `as Foo`. Narrow or validate it instead. `as const` and `satisfies` are fine, since they narrow literals without overriding the checker.
- Model mutually exclusive states as a discriminated union, not as optional fields that can contradict each other. This also feeds the exhaustive mappers above.
- Use `interface` for object shapes and `type` for unions and intersections.
- Use `undefined` for absent values, not `null`, unless an external contract requires `null`.

## Types directory (TypeScript)

- Code files hold only code. Declare every `interface` and `type` in a `types/` directory at the root of its source tree (`src/types/`, `frontend/src/types/`, `tests/types/`).
- A file left with only types moves whole into `types/`, and the empty code file is deleted (ports, definitions).
- Name every file in `types/` with the `.types.ts` suffix (`session.types.ts`), and import it with that suffix.
- Keep Zod schemas in the code file, and the types derived from them (`z.infer`, `z.input`) beside the schema. The same goes for a type derived from a runtime value, such as `(typeof ROLES)[number]`: the value is its source of truth.

## Verification

- Preserve the existing public contract unless a breaking change is intentional.
- Add or update tests around behavior and failure paths.
- Verify in order, narrowest first: the tests closest to the change, then the wider test suite, then workspace-wide type checking and linting, then the Sonar analysis. Fix any new Sonar issue on the changed code, and don't silence one without a stated reason. Run formatting before finishing.
- Keep comments focused on why a non-obvious decision exists, not on repeating what the code says.

## Applying these rules in other languages

The examples above are TypeScript. Apply the intent using the target language's idioms, and let the existing codebase's conventions win where they conflict:

- **Exhaustive mappers** — where the type system can't enforce exhaustiveness (e.g. Go), use a `map[Kind]T` plus a test that asserts every known value has an entry, or an exhaustiveness linter such as `exhaustive`.
- **`const` over `let`** — in languages without that distinction, assign each variable once; extract a function instead of reassigning across branches.
- **`Result` values** — use the language's native error convention (e.g. Go's `(T, error)`), not a custom `Result` type.
- **Named constants** — follow the language's naming style (`maxRetryCount` in Go, `MAX_RETRY_COUNT` in TypeScript/Python).
