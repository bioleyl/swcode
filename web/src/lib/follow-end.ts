/** What a scrolling element says about where it is scrolled. */
export type Scroller = Pick<Element, 'clientHeight' | 'scrollHeight' | 'scrollTop'>;

/**
 * Makes a change to what an element scrolls, keeping its end in view when people were looking at it: once they
 * scroll up they stay where they are, and following starts again when they scroll back down to the end. Within
 * `distance` pixels of the end counts as at the end.
 */
export function keepingEnd(scroller: Scroller, distance: number, change: () => void): void {
  const atEnd = scroller.scrollHeight - scroller.scrollTop - scroller.clientHeight < distance;
  change();
  if (!atEnd) {
    return;
  }
  scroller.scrollTop = scroller.scrollHeight;
}
