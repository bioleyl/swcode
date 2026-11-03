import { encode } from 'uqr';

const SVG_NAMESPACE = 'http://www.w3.org/2000/svg';
// The quiet zone a reader needs around the code, in modules.
const QUIET_ZONE = 2;

/** The dark modules of a QR code as one path: a square per module. */
const pathOf = (data: readonly (readonly boolean[])[]): string =>
  data
    .flatMap((row, y) => row.flatMap((dark, x) => (dark ? [`M${x + QUIET_ZONE} ${y + QUIET_ZONE}h1v1h-1z`] : [])))
    .join('');

/** A QR code for some text, as an SVG built by hand: it scales to any size and takes the theme's ink. */
export function qrElement(text: string, label: string): SVGSVGElement {
  const code = encode(text, { border: 0 });
  const side = code.size + QUIET_ZONE * 2;
  const svg = document.createElementNS(SVG_NAMESPACE, 'svg');
  svg.setAttribute('viewBox', `0 0 ${side} ${side}`);
  svg.setAttribute('role', 'img');
  svg.setAttribute('aria-label', label);
  svg.setAttribute('shape-rendering', 'crispEdges');
  const ground = document.createElementNS(SVG_NAMESPACE, 'rect');
  ground.setAttribute('width', String(side));
  ground.setAttribute('height', String(side));
  // A reader wants dark on light, whatever the theme.
  ground.style.fill = '#ffffff';
  const modules = document.createElementNS(SVG_NAMESPACE, 'path');
  modules.setAttribute('d', pathOf(code.data));
  modules.style.fill = '#0b0b0c';
  svg.append(ground, modules);
  return svg;
}
