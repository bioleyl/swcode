# Review mode

Use this when asked to review, audit or critique code. Report findings; don't edit the code unless asked.

## First: refactor review or correctness review?

- **Refactor review:** the code should get cleaner, not different. Behaviour must not change. List any suggestion that would change behaviour on its own line, marked "Behaviour change, confirm with the author", and never mix it into the refactor findings.
- **Correctness review:** find bugs. Behaviour changes are in scope.

If you can't tell which one is wanted, ask before reviewing.

## Walk order

Go through rule groups A to H of [SKILL.md](../SKILL.md) against the code in scope, then the floor, then the mistakes in [agent-mistakes.md](agent-mistakes.md). Put behaviour bugs and things that will break first; style comes last.

## Evidence

A finding about behaviour is stronger with proof. Without changing anything, you may:

- run the tests, the linter and the type checker;
- call a pure function from a throwaway script outside the project (in a scratch or temp folder) to show what it returns;
- read output that already exists, such as a previous build.

Never build, format, deploy, send mail, or change files or state in the project. When a finding couldn't be confirmed, say so in it.

## Codebase conventions

Where the codebase consistently follows a convention that differs from the skill (its own constants file name, no `services/` folder, module-private constants kept inline), that is not a finding: note it once in Coverage, as `codebase convention: <what>`. The project's README and CONTRIBUTING count as its rules. A convention the codebase follows only sometimes is a finding: two styles in one codebase.

## Severity

- **Critical:** users or data affected: wrong output, lost messages, a weakened secret or another security gap, a failure hidden in a way that loses data or tells the user something false, fake success, a removed floor item.
- **Important:** a maintenance cost, or a bug with no harm yet: a business rule inside a component, a module with two reasons to change, a subclass that breaks its parent's contract, speculative abstraction, I/O inside business rules, `switch` instead of a mapper, functions past the size limits, `any` or casts on outside data, vague names, missing tests for a failure path.
- **Nit:** style the linter doesn't catch.

A deliberate fallback that the function's doc comment describes, and that loses nothing, is not a finding.

## What makes a finding

Each finding gives the place, a label, what is wrong with the code quoted, and a concrete fix. With no quote or no fix, it isn't a finding: drop it.

The label is one of:

- the rule's name, such as **Don't hide failures**;
- **Floor: <name>** for a floor item that was removed or is missing, such as **Floor: security**;
- **Correctness** for a bug no rule names, such as a wrong address or a wrong calculation.

Examples:

- Good: "`src/api.ts:42` — **Don't hide failures**: `catch { return []; }` makes a network error look like an empty list. Fix: let it propagate, or return `{ ok: false, error }`."
- Good: "`src/seo.ts:63` — **Correctness**: the page's `about` points at `https://example.com/#product`, but the node's `@id` is built as `https://example.com#product`, without the slash. Fix: build both with the same helper."
- Not a finding: "The error handling could be more specific."

## Format

```
# Review: <scope>

## Summary
<2–3 sentences: ship / needs work / rewrite>
Counts: <N> critical, <M> important, <K> nits

## Critical
- `<file>:<line>` — **<label>**: <what's wrong> [`<quoted code>`]. Fix: <change>.

## Important
- …

## Nits
- <at most 3 in full>
- More: `<file>:<line>` <label>; `<file>:<line>` <label>; …

## What's good
<0–3 specific points>

## Coverage
- A. Before writing: <findings, or "clean">
- B. Names and constants: …
- C. Design (SOLID): …
- D. Frontend: <findings, "clean", or "no frontend code">
- E. Control flow and function shape: …
- F. Errors and types: …
- G. Honesty and clean-up: …
- H. Refactoring and verification: …
- Floor: …
- Tools: <linters, formatters and type checkers run; languages that have none in the project>
- Codebase conventions: <differences noted, or "none">
```

- Leave out a severity heading with no findings, and "What's good" when nothing stands out: never invent praise.
- Write at most 3 nits in full; list the rest on the "More" line, so nothing is silently dropped. The nit count includes them.
- The counts must match the findings listed.
- Fill every coverage line. A blank line is a claim you didn't check.
- Never give a score, a percentage or a "maintainability index": there is nothing to measure it against, so it would be invented.

## Settled exceptions

A `style exception` comment that names the rule, the reason and a revisit condition is settled: mention it in Coverage as "documented exception", not as a finding. One without a revisit condition is an Important finding.

## When the author disagrees

Give the reason behind the rule, from the references. If they have a reason specific to this case, suggest a `style exception` comment rather than arguing.

## What review mode doesn't do

- Change anything: no edits, builds, formatting, deploys or mail (see Evidence for what you may run).
- Replace the linter or the formatter.
- Treat missing tests as proof the code works: missing tests are a finding ("No test for an empty cart").
