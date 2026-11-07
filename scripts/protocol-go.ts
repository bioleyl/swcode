// npm run protocol: the Go daemon's protocol types, generated from the zod schemas the app parses with
// (shared/protocol), so the two sides cannot drift apart. `--check` fails when the Go file is not up to date.
import { execFileSync } from 'node:child_process';
import { readFileSync, writeFileSync } from 'node:fs';
import { resolve } from 'node:path';

import { z } from 'zod';

import * as choices from '../shared/protocol/choices.schema.js';
import * as desktop from '../shared/protocol/desktop.schema.js';
import * as endpoints from '../shared/protocol/endpoints.js';
import * as messages from '../shared/protocol/messages.schema.js';
import * as models from '../shared/protocol/models.schema.js';
import * as pairing from '../shared/protocol/pairing.schema.js';
import * as picture from '../shared/protocol/picture.schema.js';
import * as project from '../shared/protocol/project.schema.js';
import * as session from '../shared/protocol/session.schema.js';
import * as settings from '../shared/protocol/settings.schema.js';
import * as skill from '../shared/protocol/skill.schema.js';
import * as startup from '../shared/protocol/startup.schema.js';
import * as update from '../shared/protocol/update.schema.js';

const OUTPUT = resolve(import.meta.dirname, '../daemon/protocol/protocol_gen.go');
const CHECK_FLAG = '--check';
// In the order they import each other: a schema is named where it is exported first.
const MODULES: readonly Record<string, unknown>[] = [
  choices,
  picture,
  project,
  skill,
  session,
  messages,
  models,
  settings,
  update,
  pairing,
  startup,
  desktop,
  endpoints,
];
const SCHEMA_SUFFIX = 'Schema';
// Go's initialisms, as its own names write them.
const INITIALISMS: Readonly<Record<string, string>> = {
  api: 'API',
  html: 'HTML',
  http: 'HTTP',
  id: 'ID',
  json: 'JSON',
  lan: 'LAN',
  ok: 'OK',
  ssh: 'SSH',
  url: 'URL',
  urls: 'URLs',
  wsl: 'WSL',
};
const BOOLEAN_VARIANTS: Readonly<Record<string, string>> = { false: 'Failed', true: 'Ok' };

/** How a value travels: the Go type of a field, and what its checks and defaults need. */
type GoType =
  | { kind: 'string' }
  | { kind: 'int' }
  | { kind: 'float' }
  | { kind: 'bool' }
  | { kind: 'enum'; name: string }
  | { kind: 'struct'; name: string }
  | { kind: 'union'; name: string }
  | { kind: 'array'; of: GoType }
  | { kind: 'map'; key: GoType; of: GoType };

interface Checks {
  trim: boolean;
  min: number | undefined;
  max: number | undefined;
  pattern: RegExp | undefined;
  patternMessage: string | undefined;
  positive: boolean;
  // A number's bounds, both included: min(0), max(20).
  lowest: number | undefined;
  highest: number | undefined;
}

interface Field {
  json: string;
  go: string;
  type: GoType;
  optional: boolean;
  defaultValue: unknown;
  checks: Checks;
  /** The checks of each item of an array of strings. */
  itemChecks: Checks | undefined;
}

interface StructType {
  kind: 'struct';
  name: string;
  fields: Field[];
  /** Set on a union's variant: its discriminator and the value that names it. */
  variant: { key: string; value: string | boolean } | undefined;
}

interface UnionType {
  kind: 'union';
  name: string;
  key: string;
  variants: { value: string | boolean; method: string; struct: string }[];
}

interface EnumType {
  kind: 'enum';
  name: string;
  values: string[];
}

type Declaration = StructType | UnionType | EnumType;

interface ZodDef {
  type: string;
  [key: string]: unknown;
}

const defOf = (schema: unknown): ZodDef => (schema as { _zod: { def: ZodDef } })._zod.def;

const isSchema = (value: unknown): value is z.ZodType => value instanceof z.ZodType;

const words = (name: string): string[] =>
  name
    .replace(/([a-z0-9])([A-Z])/g, '$1 $2')
    .split(/[^A-Za-z0-9]+/)
    .filter((word) => word !== '');

const pascal = (name: string): string =>
  words(name)
    .map((word) => INITIALISMS[word.toLowerCase()] ?? `${word[0]?.toUpperCase()}${word.slice(1).toLowerCase()}`)
    .join('');

/** A field's Go name: its words in capitals, `sessionId` as SessionID. */
const fieldName = (name: string): string =>
  words(name)
    .map((word) => INITIALISMS[word.toLowerCase()] ?? `${word[0]?.toUpperCase()}${word.slice(1)}`)
    .join('');

const singular = (name: string): string => {
  if (/ies$/i.test(name)) {
    return `${name.slice(0, -3)}${name.endsWith('IES') ? 'Y' : 'y'}`;
  }
  return /ses$/i.test(name) ? name.slice(0, -2) : name.replace(/s$/i, '');
};

const declarations = new Map<string, Declaration>();
// The exported schemas' names, known before any is walked: one met inside another first keeps its own name.
const named = new Map<unknown, string>();
// The objects and unions already declared, by schema.
const built = new Map<unknown, string>();
const enumsByValues = new Map<string, string>();

const valuesKey = (values: readonly string[]): string => JSON.stringify([...values].sort());

function declare(declaration: Declaration): void {
  if (declarations.has(declaration.name)) {
    throw new Error(`Two Go types named ${declaration.name}`);
  }
  declarations.set(declaration.name, declaration);
}

function enumType(values: string[], fallbackName: string): GoType {
  const known = enumsByValues.get(valuesKey(values));
  if (known !== undefined) {
    return { kind: 'enum', name: known };
  }
  declare({ kind: 'enum', name: fallbackName, values });
  enumsByValues.set(valuesKey(values), fallbackName);
  return { kind: 'enum', name: fallbackName };
}

const NO_CHECKS: Checks = {
  highest: undefined,
  lowest: undefined,
  max: undefined,
  min: undefined,
  pattern: undefined,
  patternMessage: undefined,
  positive: false,
  trim: false,
};

/** A check's own message, when the schema gives one (a folder name's pattern). */
const messageOf = (check: Record<string, unknown>): string | undefined => {
  if (typeof check.error !== 'function') {
    return undefined;
  }
  const message: unknown = (check.error as (issue: object) => unknown)({});
  return typeof message === 'string' ? message : undefined;
};

function checksOf(def: ZodDef): Checks {
  const checks = (def.checks as { _zod: { def: Record<string, unknown> } }[] | undefined) ?? [];
  return checks
    .map((check) => check._zod.def)
    .reduce<Checks>((found, check) => {
      const tx = check.tx as ((text: string) => string) | undefined;
      const facts: Record<string, () => Partial<Checks>> = {
        // min(n) is greater_than n inclusive; positive() is greater_than 0, exclusive.
        greater_than: () => (check.inclusive ? { lowest: check.value as number } : { positive: check.value === 0 }),
        less_than: () => (check.inclusive ? { highest: check.value as number } : {}),
        max_length: () => ({ max: check.maximum as number }),
        min_length: () => ({ min: check.minimum as number }),
        overwrite: () => ({ trim: tx?.(' x ') === 'x' }),
        string_format: () =>
          check.format === 'regex' ? { pattern: check.pattern as RegExp, patternMessage: messageOf(check) } : {},
      };
      return { ...found, ...(facts[String(check.check)]?.() ?? {}) };
    }, NO_CHECKS);
}

const isInt = (def: ZodDef): boolean =>
  ((def.checks as { _zod: { def: Record<string, unknown> } }[] | undefined) ?? []).some(
    (check) => check._zod.def.format === 'safeint'
  );

/** What a field holds once its optional and default wrappers are taken off. */
function unwrapped(schema: unknown): { inner: unknown; optional: boolean; defaultValue: unknown } {
  const def = defOf(schema);
  if (def.type === 'optional') {
    return { ...unwrapped(def.innerType), optional: true };
  }
  if (def.type === 'default') {
    return { ...unwrapped(def.innerType), defaultValue: def.defaultValue };
  }
  return { defaultValue: undefined, inner: schema, optional: false };
}

function typeOf(schema: unknown, context: string): GoType {
  const known = named.get(schema);
  const def = defOf(schema);
  const declared = built.get(schema);
  if (declared !== undefined) {
    return { kind: def.type === 'union' ? 'union' : 'struct', name: declared };
  }
  const types: Record<string, () => GoType> = {
    array: () => ({ kind: 'array', of: typeOf(def.element, singular(context)) }),
    boolean: () => ({ kind: 'bool' }),
    enum: () => enumType(Object.values(def.entries as Record<string, string>), known ?? context),
    number: () => (isInt(def) ? { kind: 'int' } : { kind: 'float' }),
    object: () => ({ kind: 'struct', name: structOf(schema, known ?? context, undefined) }),
    record: () => ({ key: typeOf(def.keyType, `${context}Key`), kind: 'map', of: typeOf(def.valueType, context) }),
    string: () => ({ kind: 'string' }),
    union: () => ({ kind: 'union', name: unionOf(schema, known ?? context) }),
    unknown: () => ({ kind: 'string' }),
  };
  const make = types[def.type];
  if (!make) {
    throw new Error(`${context}: zod ${def.type} has no Go type`);
  }
  return make();
}

function fieldOf(key: string, schema: unknown, parent: string): Field {
  const { inner, optional, defaultValue } = unwrapped(schema);
  const innerDef = defOf(inner);
  const type = typeOf(inner, `${parent}${pascal(key)}`);
  const itemChecks = innerDef.type === 'array' ? checksOf(defOf(innerDef.element)) : undefined;
  return { checks: checksOf(innerDef), defaultValue, go: fieldName(key), itemChecks, json: key, optional, type };
}

function structOf(schema: unknown, name: string, variant: StructType['variant']): string {
  built.set(schema, name);
  const shape = defOf(schema).shape as Record<string, unknown>;
  const struct: StructType = { fields: [], kind: 'struct', name, variant };
  declare(struct);
  struct.fields = Object.entries(shape)
    .filter(([key]) => key !== variant?.key)
    .map(([key, field]) => fieldOf(key, field, name));
  return name;
}

function unionOf(schema: unknown, name: string): string {
  built.set(schema, name);
  const def = defOf(schema);
  const key = def.discriminator as string;
  const union: UnionType = { key, kind: 'union', name, variants: [] };
  declare(union);
  union.variants = (def.options as unknown[]).map((option) => {
    const value = (defOf((defOf(option).shape as Record<string, unknown>)[key]).values as (string | boolean)[])[0];
    if (value === undefined) {
      throw new Error(`${name}: a variant without its ${key}`);
    }
    const method = typeof value === 'boolean' ? (BOOLEAN_VARIANTS[String(value)] ?? '') : pascal(value);
    return { method, struct: structOf(option, `${name}${method}`, { key, value }), value };
  });
  return name;
}

// Go source.

const goTypeText = (type: GoType): string => {
  const texts: { [Kind in GoType['kind']]: (type: Extract<GoType, { kind: Kind }>) => string } = {
    array: (array) => `[]${goTypeText(array.of)}`,
    bool: () => 'bool',
    enum: (each) => each.name,
    float: () => 'float64',
    int: () => 'int',
    map: (map) => `map[${goTypeText(map.key)}]${goTypeText(map.of)}`,
    string: () => 'string',
    struct: (each) => each.name,
    union: (each) => each.name,
  };
  return (texts[type.kind] as (type: GoType) => string)(type);
};

/** An optional object or union is a pointer: `omitempty` leaves a struct in. */
const isPointer = (field: Field): boolean =>
  field.optional && (field.type.kind === 'struct' || field.type.kind === 'union');

const fieldText = (field: Field): string => {
  const type = `${isPointer(field) ? '*' : ''}${goTypeText(field.type)}`;
  return `\t${field.go} ${type} \`json:"${field.json}${field.optional ? ',omitempty' : ''}"\``;
};

/** Arrays and maps the app requires: Go writes a nil one as null, which zod refuses. */
const needsEmpty = (field: Field): boolean =>
  !field.optional && (field.type.kind === 'array' || field.type.kind === 'map');

const emptyValue = (field: Field): string => `${goTypeText(field.type)}{}`;

const goLiteral = (value: unknown, type: GoType): string => {
  if (type.kind === 'union') {
    const union = declarations.get(type.name) as UnionType;
    const key = (value as Record<string, string>)[union.key];
    const variant = union.variants.find((each) => each.value === key);
    return `${type.name}{Value: ${variant?.struct}{}}`;
  }
  if (type.kind === 'array' || type.kind === 'map') {
    return `${goTypeText(type)}{}`;
  }
  return JSON.stringify(value);
};

const quoted = (text: string): string => JSON.stringify(text);

const patternName = (pattern: RegExp): string => {
  const known = patterns.get(pattern.source);
  if (known !== undefined) {
    return known;
  }
  const name = `pattern${patterns.size + 1}`;
  patterns.set(pattern.source, name);
  return name;
};
const patterns = new Map<string, string>();

/** The checks of one string: trimmed first, as zod does, then its length and its pattern. */
function stringChecks(target: string, label: string, checks: Checks, optional: boolean): string[] {
  const lines = checks.trim ? [`\t${target} = strings.TrimSpace(${target})`] : [];
  const guard = optional ? `${target} != "" && ` : '';
  const length = `utf8.RuneCountInString(${target})`;
  if (checks.min !== undefined) {
    const message = checks.min === 1 ? 'must not be empty' : `at least ${checks.min} characters`;
    lines.push(
      `\tif ${optional ? `${target} != "" && ` : ''}${length} < ${checks.min} {\n\t\treturn fmt.Errorf("%s: ${message}", ${label})\n\t}`
    );
  }
  if (checks.max !== undefined) {
    lines.push(`\tif ${length} > ${checks.max} {\n\t\treturn fmt.Errorf("%s: at most ${checks.max} characters", ${label})\n\t}`);
  }
  if (checks.pattern !== undefined) {
    const message = quoted(checks.patternMessage ?? `does not match ${checks.pattern.source}`);
    lines.push(
      `\tif ${guard}!${patternName(checks.pattern)}.MatchString(${target}) {\n\t\treturn fmt.Errorf("%s: %s", ${label}, ${message})\n\t}`
    );
  }
  return lines;
}

function fieldChecks(field: Field): string[] {
  const target = `v.${field.go}`;
  const label = quoted(field.json);
  const type = field.type;
  if (type.kind === 'string') {
    return stringChecks(target, label, field.checks, field.optional);
  }
  if (type.kind === 'int' || type.kind === 'float') {
    return numberChecks(target, label, field.checks, field.optional);
  }
  if (type.kind === 'enum') {
    const guard = field.optional ? `${target} != "" && ` : '';
    return [`\tif ${guard}!${target}.Valid() {\n\t\treturn fmt.Errorf("%s: unknown value %q", ${label}, ${target})\n\t}`];
  }
  if (type.kind !== 'array') {
    return [];
  }
  const lines: string[] = [];
  if (field.checks.min !== undefined) {
    lines.push(`\tif len(${target}) < ${field.checks.min} {\n\t\treturn fmt.Errorf("%s: at least ${field.checks.min}", ${label})\n\t}`);
  }
  if (field.checks.max !== undefined) {
    lines.push(`\tif len(${target}) > ${field.checks.max} {\n\t\treturn fmt.Errorf("%s: at most ${field.checks.max}", ${label})\n\t}`);
  }
  const items = field.itemChecks;
  if (type.of.kind === 'string' && items && hasChecks(items)) {
    const inner = stringChecks(`${target}[index]`, label, items, false).map((line) => line.replaceAll('\n', '\n\t'));
    lines.push(`\tfor index := range ${target} {\n\t${inner.join('\n\t')}\n\t}`);
  }
  return lines;
}

// An optional number left out is 0 in Go: positive() checks one that is there.
function numberChecks(target: string, label: string, checks: Checks, optional: boolean): string[] {
  const lines: string[] = [];
  if (checks.positive) {
    const guard = optional ? `${target} != 0 && ` : '';
    lines.push(`\tif ${guard}${target} <= 0 {\n\t\treturn fmt.Errorf("%s: must be positive", ${label})\n\t}`);
  }
  if (checks.lowest !== undefined) {
    lines.push(`\tif ${target} < ${checks.lowest} {\n\t\treturn fmt.Errorf("%s: at least ${checks.lowest}", ${label})\n\t}`);
  }
  if (checks.highest !== undefined) {
    lines.push(`\tif ${target} > ${checks.highest} {\n\t\treturn fmt.Errorf("%s: at most ${checks.highest}", ${label})\n\t}`);
  }
  return lines;
}

const hasChecks = (checks: Checks): boolean =>
  checks.lowest !== undefined ||
  checks.highest !== undefined ||
  checks.trim ||
  checks.min !== undefined ||
  checks.max !== undefined ||
  checks.pattern !== undefined ||
  checks.positive;

function structText(struct: StructType): string {
  const parts = [`type ${struct.name} struct {\n${struct.fields.map(fieldText).join('\n')}\n}`];
  const empties = struct.fields.filter(needsEmpty);
  if (struct.variant || empties.length > 0) {
    const fill = empties.map((field) => `\tif p.${field.go} == nil {\n\t\tp.${field.go} = ${emptyValue(field)}\n\t}`);
    const value = struct.variant
      ? `struct {\n\t\tKey ${typeof struct.variant.value === 'boolean' ? 'bool' : 'string'} \`json:"${struct.variant.key}"\`\n\t\tplain\n\t}{${JSON.stringify(struct.variant.value)}, p}`
      : 'p';
    parts.push(
      `func (v ${struct.name}) MarshalJSON() ([]byte, error) {\n\ttype plain ${struct.name}\n\tp := plain(v)\n${fill.map((line) => `${line}\n`).join('')}\treturn json.Marshal(${value})\n}`
    );
  }
  const checks = struct.fields.flatMap(fieldChecks);
  const defaults = struct.fields.filter((field) => field.defaultValue !== undefined);
  if (checks.length > 0 || defaults.length > 0) {
    const preset = defaults.map((field) => `${field.go}: ${goLiteral(field.defaultValue, field.type)}`).join(', ');
    parts.push(
      `func (v *${struct.name}) UnmarshalJSON(data []byte) error {\n\ttype plain ${struct.name}\n\tp := plain{${preset}}\n\tif err := json.Unmarshal(data, &p); err != nil {\n\t\treturn err\n\t}\n\t*v = ${struct.name}(p)\n\treturn v.Normalize()\n}`
    );
    parts.push(
      `// Normalize trims and checks what the app sent, as its schema does.\nfunc (v *${struct.name}) Normalize() error {\n${checks.map((line) => `${line}\n`).join('')}\treturn nil\n}`
    );
  }
  return parts.join('\n\n');
}

function unionText(union: UnionType): string {
  const marker = `is${union.name}`;
  const cases = union.variants.map(
    (variant) =>
      `\tcase ${quoted(JSON.stringify(variant.value))}:\n\t\tvar value ${variant.struct}\n\t\tif err := json.Unmarshal(data, &value); err != nil {\n\t\t\treturn err\n\t\t}\n\t\tu.Value = value`
  );
  const mapCases = union.variants.map(
    (variant) => `\tcase ${variant.struct}:\n\t\treturn m.${variant.method}(value)`
  );
  return [
    `// ${union.name} holds one of its variants, told apart by "${union.key}".\ntype ${union.name} struct {\n\tValue ${union.name}Variant\n}`,
    `type ${union.name}Variant interface {\n\t${marker}()\n}`,
    ...union.variants.map((variant) => `func (${variant.struct}) ${marker}() {}`),
    `func (u ${union.name}) MarshalJSON() ([]byte, error) {\n\tif u.Value == nil {\n\t\treturn nil, errors.New("protocol: an empty ${union.name}")\n\t}\n\treturn json.Marshal(u.Value)\n}`,
    `func (u *${union.name}) UnmarshalJSON(data []byte) error {\n\tvar head struct {\n\t\tKey json.RawMessage \`json:"${union.key}"\`\n\t}\n\tif err := json.Unmarshal(data, &head); err != nil {\n\t\treturn err\n\t}\n\tswitch string(head.Key) {\n${cases.join('\n')}\n\tdefault:\n\t\treturn fmt.Errorf("protocol: no ${union.name} with ${union.key} %s", head.Key)\n\t}\n\treturn nil\n}`,
    `// ${union.name}Mapper has a method per variant: a new variant does not compile until its mappers have one.\ntype ${union.name}Mapper[R any] interface {\n${union.variants.map((variant) => `\t${variant.method}(${variant.struct}) R`).join('\n')}\n}`,
    `// Map${union.name} calls the mapper's method for the variant u holds.\nfunc Map${union.name}[R any](u ${union.name}, m ${union.name}Mapper[R]) R {\n\tswitch value := u.Value.(type) {\n${mapCases.join('\n')}\n\t}\n\tpanic(fmt.Sprintf("protocol: an empty or unknown ${union.name} %T", u.Value))\n}`,
  ].join('\n\n');
}

function enumText(each: EnumType): string {
  const constants = each.values.map((value) => `\t${each.name}${pascal(value)} ${each.name} = ${quoted(value)}`);
  return [
    `type ${each.name} string`,
    `const (\n${constants.join('\n')}\n)`,
    `// Valid tells whether v is one of the ${each.name} values.\nfunc (v ${each.name}) Valid() bool {\n\tswitch v {\n\tcase ${each.values.map((value) => `${each.name}${pascal(value)}`).join(', ')}:\n\t\treturn true\n\t}\n\treturn false\n}`,
  ].join('\n\n');
}

const declarationText = (declaration: Declaration): string => {
  const texts: { [Kind in Declaration['kind']]: (each: Extract<Declaration, { kind: Kind }>) => string } = {
    enum: enumText,
    struct: structText,
    union: unionText,
  };
  return (texts[declaration.kind] as (each: Declaration) => string)(declaration);
};

/** A constant of a module: a number, a string, a list of an enum's values, a table of strings, a pattern. */
function constantText(exportName: string, value: unknown): string | undefined {
  const name = pascal(exportName);
  if (typeof value === 'number' || typeof value === 'string') {
    return `const ${name} = ${JSON.stringify(value)}`;
  }
  if (value instanceof RegExp) {
    return `var ${name} = regexp.MustCompile(${quoted(value.source)})`;
  }
  if (Array.isArray(value) && value.every((item) => typeof item === 'string')) {
    const type = enumsByValues.get(valuesKey(value)) ?? 'string';
    const items = value.map((item) => (type === 'string' ? quoted(item) : `${type}${pascal(item)}`));
    return `var ${name} = []${type}{${items.join(', ')}}`;
  }
  // A table of strings, named in the plural (ENDPOINTS): LOCAL_HOST is a value of a schema, not a table.
  const table = typeof value === 'object' && value !== null && exportName.endsWith('S');
  if (table && Object.values(value).every((item) => typeof item === 'string')) {
    const prefix = pascal(singular(exportName));
    const lines = Object.entries(value as Record<string, string>).map(
      ([key, item]) => `\t${prefix}${pascal(key)} = ${quoted(item)}`
    );
    return `const (\n${lines.join('\n')}\n)`;
  }
  return undefined;
}

function generate(): string {
  const exports = MODULES.flatMap((module) => Object.entries(module));
  // The schemas' names first, then the enums of exported lists, then every schema's types.
  for (const [exportName, value] of exports) {
    if (isSchema(value) && exportName.endsWith(SCHEMA_SUFFIX) && defOf(value).type === 'enum') {
      typeOf(value, exportName.slice(0, -SCHEMA_SUFFIX.length));
    }
  }
  for (const [exportName, value] of exports) {
    if (Array.isArray(value) && value.length > 0 && value.every((item) => typeof item === 'string')) {
      enumType([...value], pascal(singular(exportName)));
    }
  }
  const schemas = exports.flatMap(([exportName, value]) =>
    isSchema(value) && exportName.endsWith(SCHEMA_SUFFIX)
      ? [{ name: exportName.slice(0, -SCHEMA_SUFFIX.length), schema: value }]
      : []
  );
  for (const { name, schema } of schemas) {
    named.set(schema, name);
  }
  for (const { name, schema } of schemas) {
    typeOf(schema, name);
  }
  const constants = exports.flatMap(([exportName, value]) => {
    const text = isSchema(value) ? undefined : constantText(exportName, value);
    return text === undefined ? [] : [text];
  });
  const body = [...declarations.values()].map(declarationText);
  const patternLines = [...patterns].map(([source, name]) => `var ${name} = regexp.MustCompile(${quoted(source)})`);
  return `${[
    '// Code generated by scripts/protocol-go.ts from shared/protocol; DO NOT EDIT.',
    '',
    'package protocol',
    '',
    'import (\n\t"encoding/json"\n\t"errors"\n\t"fmt"\n\t"regexp"\n\t"strings"\n\t"unicode/utf8"\n)',
    '',
    'var (\n\t_ = errors.New\n\t_ = strings.TrimSpace\n\t_ = utf8.RuneCountInString\n)',
    '',
    [...constants, ...patternLines].join('\n\n'),
    '',
    body.join('\n\n'),
  ].join('\n')}\n`;
}

// gofmt lines the fields up, as Go's own files are.
const text = execFileSync('gofmt', [], { encoding: 'utf8', input: generate() });
if (process.argv.includes(CHECK_FLAG)) {
  const current = readFileSync(OUTPUT, 'utf8');
  if (current !== text) {
    console.error('daemon/protocol/protocol_gen.go is not up to date: run npm run protocol.');
    process.exit(1);
  }
} else {
  writeFileSync(OUTPUT, text);
}
