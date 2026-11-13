const TAX = 0.08;

/** A price with the tax on top, rounded to a whole number. */
export const calcPrice = (price) => Math.round(price * (1 + TAX));
