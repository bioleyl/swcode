/** The options of the command, as its help lists them. */
export const OPTIONS = [
  { default: '8080', flag: '--port', says: 'The port to listen on' },
  { default: '127.0.0.1', flag: '--host', says: 'The address to listen on' },
  { default: 'false', flag: '--quiet', says: 'Print nothing but errors' },
];
