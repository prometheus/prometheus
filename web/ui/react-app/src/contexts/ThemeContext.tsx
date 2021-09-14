import React from 'react';

export type themeName = 'light' | 'dark';
export type themeSetting = themeName | 'auto';

export interface ThemeCtx {
  theme: themeName;
  userPreference: themeSetting;
  setTheme: (t: themeSetting) => void;
}

// defaults, will be overriden in App.tsx
export const ThemeContext = React.createContext<ThemeCtx>({
  theme: 'light',
  userPreference: 'auto',
  // eslint-disable-next-line @typescript-eslint/no-empty-function
  setTheme: (s: themeSetting) => {},
});

export const useTheme = (): ThemeCtx => {
  return React.useContext(ThemeContext);
};
