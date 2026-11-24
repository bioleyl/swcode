import { DEFAULT_PORT, MAX_PORT, PORT_ENV } from '../swcode.constants';

const COMMENT_MARK = '#';
const EXPORT_PREFIX = 'export ';
const QUOTES = ["'", '"', '`'];
const ESCAPED_NEWLINE = '\\n';
const NEWLINE = '\n';
const PORT_DIGITS = /^\d+$/;
const EQUALS_MARK = '=';

/** A line of the settings file: `KEY=value`, `export KEY=value`, a comment, or nothing. */
interface Setting {
  key: string;
  value: string;
}

/** The quotes the settings writer may put around a value; the same set daemon/config/settings.go uses. */
function unquoted(value: string): string {
  const opening = value[0] ?? '';
  if (!QUOTES.includes(opening) || value.at(-1) !== opening || value.length < 2) {
    return value;
  }
  return value.slice(1, -1).replaceAll(ESCAPED_NEWLINE, NEWLINE);
}

function settingOf(line: string): Setting | undefined {
  const trimmed = line.trim();
  if (trimmed === '' || trimmed.startsWith(COMMENT_MARK)) {
    return undefined;
  }
  const assignment = trimmed.indexOf(EQUALS_MARK);
  if (assignment < 0) {
    return undefined;
  }
  const key = trimmed.slice(trimmed.startsWith(EXPORT_PREFIX) ? EXPORT_PREFIX.length : 0, assignment).trim();
  if (key === '') {
    return undefined;
  }
  return { key, value: unquoted(trimmed.slice(assignment + EQUALS_MARK.length)) };
}

/**
 * The settings file's text (`~/.swcode/settings.env`) as key/value pairs. The file keeps the last value saved for
 * a key, so a later line wins over an earlier one.
 */
export function parseSettingsEnv(text: string): Record<string, string> {
  const settings: Record<string, string> = {};
  for (const line of text.split(NEWLINE)) {
    const setting = settingOf(line);
    if (setting) {
      settings[setting.key] = setting.value;
    }
  }
  return settings;
}

/** The port the daemon listens on: `SWCODE_PORT` when the settings hold a usable one, the default otherwise. */
export function portOf(settings: Record<string, string>): number {
  const saved = settings[PORT_ENV] ?? '';
  if (!PORT_DIGITS.test(saved)) {
    return DEFAULT_PORT;
  }
  const port = Number(saved);
  return port > 0 && port <= MAX_PORT ? port : DEFAULT_PORT;
}
