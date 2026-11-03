const THOUSAND = 1000;
const FULL = 100;
const MS_PER_MINUTE = 60_000;
const MINUTES_PER_HOUR = 60;
const HOURS_PER_DAY = 24;
const TRAILING_ZERO = /\.0$/;
// A project's path is in its host's style: POSIX, or Windows' backslashes.
const PATH_SEPARATORS = /[\\/]/;

/** A token count as the panels show it: 940, 24.1k, 64k. */
export const tokens = (count: number): string =>
  count < THOUSAND ? String(count) : `${(count / THOUSAND).toFixed(1).replace(TRAILING_ZERO, '')}k`;

/** How much of a whole a part is, from 0 to 100. */
export const percent = (part: number, whole: number): number =>
  whole <= 0 ? 0 : Math.min(FULL, Math.round((part / whole) * FULL));

/** How full the context is, in words: "24.1k / 64k", or only the tokens when the context's size is unknown. */
export function contextText(used: number, capacity: number | undefined): string {
  if (used === 0) {
    return '';
  }
  return capacity ? `${tokens(used)} / ${tokens(capacity)}` : `${tokens(used)} tokens`;
}

/** How long ago something happened, in the words of a list: just now, 4 min ago, 2 h ago, or the date. */
export function ago(iso: string, now: Date): string {
  const minutes = Math.floor((now.getTime() - new Date(iso).getTime()) / MS_PER_MINUTE);
  if (minutes < 1) {
    return 'just now';
  }
  if (minutes < MINUTES_PER_HOUR) {
    return `${minutes} min ago`;
  }
  const hours = Math.floor(minutes / MINUTES_PER_HOUR);
  return hours < HOURS_PER_DAY ? `${hours} h ago` : new Date(iso).toLocaleDateString();
}

/** The last folder of a path, on any host: what a list of workspaces shows. */
export const folderName = (path: string): string =>
  path
    .split(PATH_SEPARATORS)
    .filter((part) => part !== '')
    .at(-1) ?? path;
