// The settings file keeps the hosts in one line, by commas; the daemon reads spaces between them as well.
const SEPARATOR = /[\s,]+/;
const JOINER = ', ';
const BLANK = /[\s,]/;

const NO_HOST = 'Type a host: user@host, or a Host of ~/.ssh/config.';
const ONE_HOST = 'One host, without spaces or commas.';
const TAKEN = 'This host is in the list already.';

/** The hosts of the setting's line. */
export const sshHostsOf = (line: string): string[] => line.split(SEPARATOR).filter((host) => host !== '');

/** The setting's line of the hosts. */
export const sshHostsLine = (hosts: readonly string[]): string => hosts.join(JOINER);

/** Why a host cannot go in the list (taken: the others), nothing when it can. */
export function sshHostProblem(host: string, taken: readonly string[]): string {
  if (host === '') {
    return NO_HOST;
  }
  if (BLANK.test(host)) {
    return ONE_HOST;
  }
  return taken.includes(host) ? TAKEN : '';
}
