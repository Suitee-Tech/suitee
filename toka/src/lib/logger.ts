type LogFn = (...args: unknown[]) => void;

const noop: LogFn = () => {};

const debug: LogFn = import.meta.env.DEV ? console.debug.bind(console) : noop;
const info: LogFn = import.meta.env.DEV ? console.info.bind(console) : noop;
const warn: LogFn = console.warn.bind(console);
const error: LogFn = console.error.bind(console);

export const logger = { debug, info, warn, error };

