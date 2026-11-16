// The settings file keeps the hosts in one line, by commas; the daemon reads spaces between them as well.
const SEPARATOR = /[\s,]+/;
const JOINER = ', ';
const BLANK = /[\s,]/;
// A name and the endpoint it stands for, as `marvin=me@marvin`: an entry without a name is named by its endpoint.
const NAMED = '=';

const NO_ENDPOINT = 'Type a host: user@host, or a Host of ~/.ssh/config.';
const NO_NAME = 'Type a name for it, or none.';
const ONE_WORD = 'One host, without spaces or commas.';
const ONE_WORD_NAME = 'A name without spaces, commas or =.';
const TAKEN = 'This host is in the list already.';
const NAME_TAKEN = 'A host of this name is in the list already.';

/** An SSH host of the settings: the name people read it by, and the endpoint ssh reaches. */
export type SshHost = { name: string; endpoint: string };

const hostOf = (entry: string): SshHost => {
  const at = entry.indexOf(NAMED);
  return at > 0 && at < entry.length - 1
    ? { endpoint: entry.slice(at + 1), name: entry.slice(0, at) }
    : { endpoint: entry, name: entry };
};

/** The hosts of the setting's line. */
export const sshHostsOf = (line: string): SshHost[] =>
  line
    .split(SEPARATOR)
    .filter((entry) => entry !== '')
    .map(hostOf);

/** The setting's line of the hosts, each named when its name is not its endpoint. */
export const sshHostsLine = (hosts: readonly SshHost[]): string =>
  hosts
    .map((host) => (host.name === host.endpoint ? host.endpoint : `${host.name}${NAMED}${host.endpoint}`))
    .join(JOINER);

/** Why a host cannot go in the list (`taken`: the others), nothing when it can. */
export function sshHostProblem(host: SshHost, taken: readonly SshHost[]): string {
  if (host.endpoint === '') {
    return NO_ENDPOINT;
  }
  if (BLANK.test(host.endpoint) || host.endpoint.includes(NAMED)) {
    return ONE_WORD;
  }
  if (host.name === '') {
    return NO_NAME;
  }
  if (BLANK.test(host.name) || host.name.includes(NAMED)) {
    return ONE_WORD_NAME;
  }
  const endpoints = taken.map((each) => each.endpoint);
  const names = taken.map((each) => each.name);
  if (endpoints.includes(host.endpoint)) {
    return TAKEN;
  }
  return names.includes(host.name) ? NAME_TAKEN : '';
}

/** How a host reads in a list: its name, and the endpoint it stands for when that is not the name. */
export const sshHostDetail = (host: SshHost): string | undefined =>
  host.name === host.endpoint ? undefined : host.endpoint;
