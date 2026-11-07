# TypeScript types

Details for rules 23–25.

## No `any`, no casts on outside data (rule 23)

**Why:** a cast tells the compiler to stop checking. If the data isn't what the cast claims, the error shows up far from its cause.

```ts
// Bad
const user = (await response.json()) as User;
```

```ts
// Good: user.gateway.ts
const userSchema = z.object({
  id: z.string(),
  name: z.string(),
});
export type User = z.infer<typeof userSchema>;

const user = userSchema.parse(await response.json());
```

- Never use `any`. Use `unknown` only at boundaries (parsed JSON, network, environment, `catch`) and narrow it with a type guard or a validator before use.
- In a `catch`, the error is `unknown`: narrow it (`error instanceof Error`) before reading its fields.
- `as const` and `satisfies` are fine: they narrow literals without overriding the checker.

## States as unions (rule 24)

**Why:** optional fields allow combinations that can't happen (`done: true` with an `error`), and every reader has to guess which ones do. A discriminated union lists exactly the states that exist, and feeds the exhaustive mappers of rule 18.

```ts
// Bad
interface Upload {
  done?: boolean;
  progress?: number;
  url?: string;
  error?: string;
}
```

```ts
// Good
type Upload =
  | { status: 'uploading'; progress: number }
  | { status: 'done'; url: string }
  | { status: 'failed'; error: string };
```

- Use `interface` for object shapes, `type` for unions and intersections.
- Use `undefined` for absent values, not `null`, unless an outside contract requires `null`.

## Types in `types/` (rule 25)

**Why:** code files stay short and hold only code; every type has one predictable place.

- Code files hold only code. Every `interface` and `type` is declared in a `types/` directory, in a file named after its code file: `form.service.ts` → `form.service.types.ts`.
- **Which `types/`:** the module's own when the code is organised by module (`src/modules/forms/types/`, `frontend/src/forms/types/`), otherwise the one at the root of its source tree (`src/types/`, `tests/types/`).
- **Flat:** a `types/` directory never repeats the code's folders.
- **Types no single code file owns** (a gateway's interface, records several files share) take their subject's name: `form.gateway.types.ts`, `catalog.types.ts`.
- **A file left with only types** moves whole into `types/`, and the empty code file is deleted (gateways, definitions).
- **Inline types:** the rule covers named declarations. A short anonymous type used once, in `satisfies` or a generic argument (`satisfies ReadonlyArray<{ id: string; href: string }>`), may stay inline; give it a name in `types/` once it's reused or longer than a line.
- **Suffix:** every file in `types/` ends in `.types.ts` (`session.types.ts`), and is imported with that suffix.
- **Stays beside its value:** Zod schemas stay in the code file, with the types derived from them (`z.infer`, `z.input`) beside the schema. The same goes for any type derived from a runtime value, such as `(typeof ROLES)[number]`: the value is its source of truth.

Code organised by module:

```
src/modules/forms/
├── form.service.ts
├── form.service.constants.ts
└── types/
    ├── form.service.types.ts    ← the types of form.service.ts
    └── form.gateway.types.ts    ← a gateway interface no single file owns
```

Code not organised by module:

```
src/
├── session.ts
├── catalog.ts
└── types/
    ├── session.types.ts
    └── catalog.types.ts
tests/
└── types/
    └── fixtures.types.ts
```
