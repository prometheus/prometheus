import { DEFAULT_HEADERS, handleResponse, getOptions, fetchAPI } from './FetchAPI';

describe('FetchAPI', () => {
  it('should have properly defined default headers', () => {
    expect(DEFAULT_HEADERS).toEqual({
      Accept: 'application/json',
      'Content-Type': 'application/json',
    });
  });
  describe('FetchAPI', () => {
    const mockFetch: any = fetch;
    beforeEach(() => {
      mockFetch.resetMocks();
    });
    it('should', async () => {
      const mock = mockFetch.mockResponse(JSON.stringify({ foo: 123 }));
      const result = await fetchAPI('8a.nu');
      expect(result).toEqual(JSON.stringify({ foo: 123 }));
      expect(mock).toHaveBeenCalledWith('8a.nu', {
        credentials: 'same-origin',
        headers: {
          Accept: 'application/json',
          'Content-Type': 'application/json',
        },
      });
    });
  });
  describe('getOptions', () => {
    it('should merge options with default headers and credentials', () => {
      expect(getOptions({ foo: 123, bar: 234 } as any)).toEqual({
        bar: 234,
        credentials: 'same-origin',
        foo: 123,
        headers: {
          Accept: 'application/json',
          'Content-Type': 'application/json',
        },
      });
    });
    it('should override default headers and credentials', () => {
      const opt = getOptions({ credentials: 'include', headers: { Accept: 'text/html' } } as any);
      expect(opt).toEqual({
        credentials: 'include',
        headers: {
          Accept: 'text/html',
          'Content-Type': 'application/json',
        },
      });
    });
  });
  describe('handleResponse', () => {
    it('should handle json response properly', () => {
      const mockResponse: any = {
        headers: {
          get: (key: string) => (({ 'content-type': 'application/json' } as any)[key]),
        },
        ok: true,
        json: () => 'foo',
      };
      expect(handleResponse(mockResponse)).toEqual('foo');
    });
    it('should handle text response properly', () => {
      const mockResponse: any = {
        headers: {
          get: (key: string) => (({ 'content-type': 'text/html' } as any)[key]),
        },
        ok: true,
        text: () => 'bar',
      };
      handleResponse(mockResponse);
      expect(handleResponse(mockResponse)).toEqual('bar');
    });
    it('should throw error when OK status is false', () => {
      const mockResponse: any = {
        ok: false,
        statusText: 'error',
      };
      try {
        handleResponse(mockResponse);
      } catch (error) {
        expect(error.message).toEqual('error');
      }
    });
  });
});
