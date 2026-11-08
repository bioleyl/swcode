/**
 * A conversation is shown from its end, and its older events come as people scroll up. The rules of that window:
 * when a page of them is worth asking for, and where the scroll lands once they were added above.
 */

/** What a scrolling element says about itself while it holds a page of a conversation. */
export type WindowScroller = Pick<Element, 'clientHeight' | 'scrollHeight' | 'scrollTop'>;

/**
 * Whether the top of what a scrolling element holds is in view: the end of the events it holds, where older ones
 * would be added. `top` is how far below the top still counts as reaching it.
 */
export const atTop = (scroller: Pick<Element, 'scrollTop'>, top: number): boolean => scroller.scrollTop <= top;

/**
 * Where the scroll goes once older events were added above what a scrolling element held: back to the end for
 * those who were there, wherever the browser left it for those who were not, which keeps what they read under the
 * same line the older page pushed in.
 */
export const scrollAfterOlder = (scroller: Pick<Element, 'scrollHeight' | 'scrollTop'>, atEnd: boolean): number =>
  atEnd ? scroller.scrollHeight : scroller.scrollTop;

/**
 * Whether a conversation opened on its end should ask for older events at once: what it holds does not fill the
 * window, so people see empty space above rather than the beginning of what the session holds.
 */
export const fillsWindow = (scroller: WindowScroller, olderLeft: number): boolean =>
  olderLeft > 0 && scroller.scrollHeight < scroller.clientHeight;
