import React from 'react';
import { mount } from 'enzyme';

import useGlobalVars, { GlobalVarsProvider } from './useGlobalVars';

describe('useGlobalVars', () => {
  it('returns empty string as default if placeholder not replaced', () => {
    /* eslint-disable-next-line @typescript-eslint/no-explicit-any */
    (global as any).GLOBAL_PATH_PREFIX = 'PATH_PREFIX_PLACEHOLDER';
    /* eslint-disable-next-line @typescript-eslint/no-explicit-any */
    (global as any).GLOBAL_CONSOLES_LINK = 'CONSOLES_LINK_PLACEHOLDER';
    const TestComponent = () => {
      const { pathPrefix, consolesLink } = useGlobalVars();
      return (
        <div>
          <h1>{pathPrefix}</h1>
          <h2>{consolesLink}</h2>
        </div>
      );
    };
    const component = mount(
      <GlobalVarsProvider>
        <TestComponent />
      </GlobalVarsProvider>
    );
    expect(component.find('h1').text()).toBe('');
    expect(component.find('h2').text()).toBe('');
  });

  it('returns the variables from global scope', () => {
    /* eslint-disable-next-line @typescript-eslint/no-explicit-any */
    (global as any).GLOBAL_PATH_PREFIX = '/path/prefix';
    /* eslint-disable-next-line @typescript-eslint/no-explicit-any */
    (global as any).GLOBAL_CONSOLES_LINK = '/path/consoles';
    const TestComponent = () => {
      const { pathPrefix, consolesLink } = useGlobalVars();
      return (
        <div>
          <h1>{pathPrefix}</h1>
          <h2>{consolesLink}</h2>
        </div>
      );
    };
    const component = mount(
      <GlobalVarsProvider>
        <TestComponent />
      </GlobalVarsProvider>
    );
    expect(component.find('h1').text()).toBe('/path/prefix');
    expect(component.find('h2').text()).toBe('/path/consoles');
  });
});
