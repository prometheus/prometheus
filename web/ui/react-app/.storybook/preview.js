import React, { useState } from 'react';

import '../src/themes/app.scss';
import '../src/themes/light.scss';
import '../src/themes/dark.scss';
import '../src/globals';
import { Theme } from '../src/Theme';
import { ThemeContext } from '../src/contexts/ThemeContext';

export const parameters = {
  actions: { argTypesRegex: '.*' },
};

export const decorators = [
  (Story, context) => {
    return (
      <ThemeContext.Provider
        value={{
          theme: context.globals.theme,
          userPreference: context.globals.theme,
          setTheme: () => {},
        }}
      >
        <Theme />
        <Story />
      </ThemeContext.Provider>
    );
  },
];

export const globalTypes = {
  theme: {
    name: 'Theme',
    description: 'Global theme for components',
    defaultValue: 'light',
    toolbar: {
      icon: 'mirror',
      items: ['light', 'dark'],
    },
  },
};
