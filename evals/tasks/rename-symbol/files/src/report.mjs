import { calcPrice } from './price.mjs';

// calcPrice already rounds: the line shows it as it is.
export const reportLine = (item) => `${item.name}: ${calcPrice(item.price)}`;
