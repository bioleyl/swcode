const SVG_NAMESPACE = 'http://www.w3.org/2000/svg';
const VIEW_BOX = '-1 -1 26 30';
// The mark's width over its height, in its view box.
const ASPECT = 26 / 30;
const STROKE_WIDTH = '1.5';

/** The two halves of the mark, as in the SyWare logo: blue above, ink below, a band of the ground between. */
const HALVES = [
  { colour: 'var(--jl-brand)', points: '0,7 12,0 21.94,5.8 0,18.6' },
  { colour: 'var(--jl-text)', points: '24,9.4 24,21 12,28 2.06,22.2' },
] as const;

const halfOf = (colour: string, points: string): SVGPolygonElement => {
  const polygon = document.createElementNS(SVG_NAMESPACE, 'polygon');
  polygon.setAttribute('points', points);
  // In style, not attributes: only CSS reads custom properties, so the mark follows the theme.
  polygon.style.fill = colour;
  polygon.style.stroke = colour;
  polygon.setAttribute('stroke-width', STROKE_WIDTH);
  polygon.setAttribute('stroke-linejoin', 'round');
  return polygon;
};

/** The swcode mark, `height` pixels tall. JSX makes HTML elements only, so the SVG is built by hand. */
export function markElement(height: number): SVGSVGElement {
  const svg = document.createElementNS(SVG_NAMESPACE, 'svg');
  svg.setAttribute('viewBox', VIEW_BOX);
  svg.setAttribute('width', String(Math.round(height * ASPECT)));
  svg.setAttribute('height', String(height));
  svg.setAttribute('aria-hidden', 'true');
  svg.append(...HALVES.map((half) => halfOf(half.colour, half.points)));
  return svg;
}
