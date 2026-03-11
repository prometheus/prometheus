import { Dispatch, SetStateAction, useEffect, useState } from 'react';

export function useLocalStorage<S>(localStorageKey: string, initialState: S): [S, Dispatch<SetStateAction<S>>] {
  const [value, setValue] = useState(() =>
    JSON.parse(localStorage.getItem(localStorageKey) || JSON.stringify(initialState))
  );

  useEffect(() => {
    const serializedState = JSON.stringify(value);
    localStorage.setItem(localStorageKey, serializedState);
  }, [localStorageKey, value]);

  return [value, setValue];
}
