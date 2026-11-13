import { calcPrice } from './price.mjs';

/** What a cart costs, tax included. */
export const cartTotal = (items) => items.reduce((total, item) => total + calcPrice(item.price), 0);
