import { configure } from 'enzyme';
import Adapter from '@wojtekmaj/enzyme-adapter-react-17';
import { GlobalWithFetchMock } from 'jest-fetch-mock';
import 'mutationobserver-shim'; // Needed for CodeMirror.
import './globals';
import 'jest-canvas-mock';

configure({ adapter: new Adapter() });
const customGlobal: GlobalWithFetchMock = global as GlobalWithFetchMock;
customGlobal.fetch = require('jest-fetch-mock');
customGlobal.fetchMock = customGlobal.fetch;

// https://stackoverflow.com/questions/39830580/jest-test-fails-typeerror-window-matchmedia-is-not-a-function
// https://jestjs.io/docs/manual-mocks#mocking-methods-which-are-not-implemented-in-jsdom
Object.defineProperty(window, 'matchMedia', {
  writable: true,
  value: jest.fn().mockImplementation((query) => ({
    matches: false,
    media: query,
    onchange: null,
    addListener: jest.fn(), // Deprecated
    removeListener: jest.fn(), // Deprecated
    addEventListener: jest.fn(),
    removeEventListener: jest.fn(),
    dispatchEvent: jest.fn(),
  })),
});

// CodeMirror in the expression input requires this DOM API. When we upgrade react-scripts
// and the associated Jest deps, hopefully this won't be needed anymore.
document.getSelection = function () {
  return {
    addRange: function () {
      return;
    },
    removeAllRanges: function () {
      return;
    },
  };
};
