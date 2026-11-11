// npm run build:binary: swcode's command line as one program, the web app inside it. Go builds it
// for any platform from here: this machine's by default, others named as arguments (linux-arm64, windows-amd64…).
import { binaryOf, goBuild, HERE } from './go-build.mjs';

const COMMAND_LINE = './cmd/swcode';

const platforms = process.argv.length > 2 ? process.argv.slice(2) : [HERE];
for (const platform of platforms) {
  goBuild({ out: binaryOf(platform), platform, target: COMMAND_LINE });
  console.log(`Built ${binaryOf(platform)}.`);
}
