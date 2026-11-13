/** The value, kept between min and max. */
export const clamp = (value, min, max) => Math.min(Math.max(value, min), max);
