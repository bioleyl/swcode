/** The items of a page; pages count from 1. */
export const page = (items, number, size) => items.slice(number * size, number * size + size);

/** How many pages hold that many items. */
export const pageCount = (count, size) => Math.floor(count / size) + 1;
