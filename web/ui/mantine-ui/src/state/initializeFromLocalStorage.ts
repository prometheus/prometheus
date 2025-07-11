// This has to live in its own file since including it from
// localStorageMiddleware.ts causes startup issues, as the
// listener setup there accesses an action creator before Redux
// has been initialized.
export const initializeFromLocalStorage = <T>(
  key: string,
  defaultValue: T
): T => {
  const value = localStorage.getItem(key);
  if (value === null) {
    return defaultValue;
  }
  return JSON.parse(value);
};
