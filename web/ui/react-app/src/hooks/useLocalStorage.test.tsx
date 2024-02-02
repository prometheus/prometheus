import { useLocalStorage } from './useLocalStorage';
import { renderHook, act } from '@testing-library/react-hooks';

describe('useLocalStorage', () => {
  it('returns the initialState', () => {
    const initialState = { a: 1, b: 2 };
    const { result } = renderHook(() => useLocalStorage('mystorage', initialState));
    expect(result.current[0]).toEqual(initialState);
  });
  it('stores the initialState as serialized json in localstorage', () => {
    const key = 'mystorage';
    const initialState = { a: 1, b: 2 };
    renderHook(() => useLocalStorage(key, initialState));
    expect(localStorage.getItem(key)).toEqual(JSON.stringify(initialState));
  });
  it('returns a setValue function that can reset local storage', () => {
    const key = 'mystorage';
    const initialState = { a: 1, b: 2 };
    const { result } = renderHook(() => useLocalStorage(key, initialState));
    const newValue = { a: 2, b: 5 };
    act(() => {
      result.current[1](newValue);
    });
    expect(result.current[0]).toEqual(newValue);
    expect(localStorage.getItem(key)).toEqual(JSON.stringify(newValue));
  });
  it('localStorage.getItem calls once', () => {
    // do not prepare the initial state on every render except the first
    const spyStorage = jest.spyOn(Storage.prototype, 'getItem') as jest.Mock;

    const key = 'mystorage';
    const initialState = { a: 1, b: 2 };
    const { result } = renderHook(() => useLocalStorage(key, initialState));
    const newValue = { a: 2, b: 5 };
    act(() => {
      result.current[1](newValue);
    });
    expect(spyStorage).toHaveBeenCalledTimes(1);
    spyStorage.mockReset();
  });
});
