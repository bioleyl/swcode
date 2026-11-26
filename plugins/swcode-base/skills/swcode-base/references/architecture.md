# Architecture: concepts, not files

The rules in section I exist because a codebase can pass every file-local rule and still be unmaintainable.

## What goes wrong

- One concept is re-encoded in several files: a search criterion is parsed in the plan, routed in a text file, translated in a SQL map, projected in a write path, indexed in an FTS file, and mapped back in a response file. Changing one operator means editing five to nine files.
- Knowledge lives in parallel enumerations: the same operator list appears in constants, in a `Record<Kind, …>` per concern, in SQL maps, in UI labels and in docs. Nothing makes the compiler fail when one copy is forgotten.
- Rules are satisfied file by file: every helper is short, every file has one job, and the feature is still unreadable because no file holds the concept.
- Extracting a module becomes a move: every file survives, the folder changes, the duplication stays. The line count barely moves, which is the signal that nothing was refactored.
- A small feature costs dozens of files, so every change touches many of them and no one can review it.

## The target

1. **One concept, one owner module.** Reading the concept end to end stays inside that module: its input, its rules, its storage translation and its output. A new case is one entry there.
2. **One declaration, derived everywhere.** A union, operator list or kind table is declared once; other code derives it (`keyof typeof`, `satisfies Record<…>`, a `switch` the compiler checks). A second map over the same union is a defect unless it is the registry that owns the concept.
3. **Two backends are two translators of one generic model.** The query is backend-neutral (model fields, typed values, no table, column or storage encoding); each adapter translates it. The executor decides *which* adapter answers a condition, not *how*.
4. **A feature has a file budget.** An endpoint is a handful of files. If it needs a dozen that only make sense together, the cut is wrong: merge by concept, delete the intermediate helpers.
5. **Tests at the seam.** A feature test crosses the boundary a caller sees: request → result, command → effect. Tests that freeze private plan objects, generated SQL or helper returns are change detectors; keep one golden case where the encoding itself is the contract, delete the rest with the internals.

## Worked example: search

One criterion, before: plan file, text-routing file, SQL condition map, projection writer, FTS document builder, response mapper — six encodings, 37 files for one endpoint.

After:

```
contract.ts    query AST + request/answer schema
compile.ts     request -> query; routing (index words / SQL conditions)
postgres.ts    SQL translation + repository
fts5.ts        full-text translation (index, match, sync)
projection.ts  the stored values the SQL reads
search.ts      the use case
constants.ts
```

The query names model fields and typed values; the compiler decides once what the index answers; the two adapters only translate. Adding an operator is one entry in the operator table and one translator branch.

## Move or rewrite

| Situation | Do |
|---|---|
| Boundary is wrong, code is fine | move |
| One concept encoded N times | rewrite the concept behind pinned tests |
| God file with phases | split by phase, then rewrite the duplicated phase |
| Tests freeze internals | replace them with feature tests; delete them with the internals |

A move is not a refactor. Count files and lines before and after; if both are flat, the design did not change.

## Before a large rewrite

- Write the target shape down (types, one example) and agree it before coding.
- Name what disappears (files, maps, tables, migrations), not only what is added.
- One module per session, green at each end; never two engines at once.
- If the rewrite does not shrink after the first module, stop: the abstraction is still the old one.
