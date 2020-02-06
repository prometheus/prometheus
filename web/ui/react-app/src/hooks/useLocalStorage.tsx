import { Dispatch, SetStateAction, useEffect, useState } from 'react';

export function useLocalStorage<S>(localStorageKey: string, initialState: S): [S, Dispatch<SetStateAction<S>>] {
  const localStorageState = JSON.parse(localStorage.getItem(localStorageKey) || JSON.stringify(initialState));
  const [value, setValue] = useState(localStorageState);

  useEffect(() => {
    const serializedState = JSON.stringify(value);
    localStorage.setItem(localStorageKey, serializedState);
  }, [localStorageKey, value]);

  return [value, setValue];
}
