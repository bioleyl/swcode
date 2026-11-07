// What a phone's browser says it runs on.
const PHONE = /iPhone|Android|Mobi/;

/** Whether a browser runs on a phone, from what it says it runs on: updates are installed from a computer. */
export const isPhone = (userAgent: string): boolean => PHONE.test(userAgent);
