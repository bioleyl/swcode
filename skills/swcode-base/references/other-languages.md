# Other languages

The rules and examples are written for TypeScript. In another language, keep the intent and use the language's own idioms. Where they conflict, the existing codebase's conventions win. Check the language version the project targets before using a feature.

- **Exhaustive mappers:** where the compiler can't check exhaustiveness (Go), use a `map[Kind]T` plus a test that asserts every known value has an entry, or a linter such as `exhaustive`.
- **PHP mappers:** from PHP 8.1, `match` over an enum, with PHPStan reporting unhandled cases when the project has it. On older PHP, or without PHPStan, a constant array keyed by the cases plus a test that every case has an entry: an unknown key in an array is only a warning.
- **Failure values:** many PHP functions return `false` or `null` on failure (`file_get_contents`, `fwrite`, `preg_replace`, `json_decode`). Check the result before using it; a cast such as `(string)` turns the failure into an empty string and hides it (rule 21).
- **PHP arrays are values:** changing an array parameter inside a function doesn't reach the caller, so it isn't an output parameter. Still return the new array rather than relying on that.
- **`const` over `let`:** in languages without that distinction, assign each variable once; extract a function instead of reassigning across branches.
- **`Result` values:** use the language's native error convention (Go's `(T, error)`, exceptions in PHP and Python), not a custom `Result` type.
- **Named constants:** follow the language's naming style: `maxRetryCount` in Go, `MAX_RETRY_COUNT` in TypeScript, Python and PHP class constants. Values shared with the TypeScript side follow [constants.md](constants.md#values-shared-between-languages).
- **Constants files and `types/`:** these are TypeScript layout rules. Elsewhere, follow the language's and the project's usual layout, keeping the goal: short code files, and fixed values in one place.
- **No linter or formatter:** when the project has none for a language, say so in the self-check report or the review's Coverage instead of skipping silently, and match the style of the surrounding files by hand.
- **Everything else** (reading first, size limits, guard clauses, error handling, the agent mistakes, verification) applies as written.
