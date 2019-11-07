import useFetches from './useFetches';
import { renderHook } from '@testing-library/react-hooks';

describe('useFetches', () => {
  beforeEach(() => {
    fetchMock.resetMocks();
  });
  it('should can handle multiple requests', async done => {
    fetchMock.mockResponse(JSON.stringify({ satus: 'success', data: { id: 1 } }));
    const { result, waitForNextUpdate } = renderHook(useFetches, { initialProps: ['/foo/bar', '/foo/bar', '/foo/bar'] });
    await waitForNextUpdate();
    expect(result.current.response).toHaveLength(3);
    done();
  });
  it('should can handle success flow -> isLoading=true, response=[data, data], isLoading=false', async done => {
    fetchMock.mockResponse(JSON.stringify({ satus: 'success', data: { id: 1 } }));
    const { result, waitForNextUpdate } = renderHook(useFetches, { initialProps: ['/foo/bar'] });
    expect(result.current.isLoading).toEqual(true);
    await waitForNextUpdate();
    expect(result.current.response).toHaveLength(1);
    expect(result.current.isLoading).toEqual(false);
    done();
  });
  it('should isLoading remains true on empty response', async done => {
    fetchMock.mockResponse(jest.fn());
    const { result, waitForNextUpdate } = renderHook(useFetches, { initialProps: ['/foo/bar'] });
    expect(result.current.isLoading).toEqual(true);
    await waitForNextUpdate();
    setTimeout(() => {
      expect(result.current.isLoading).toEqual(true);
      done();
    }, 1000);
  });
  it('should set error message when response fail', async done => {
    fetchMock.mockReject(new Error('errr'));
    const { result, waitForNextUpdate } = renderHook(useFetches, { initialProps: ['/foo/bar'] });
    expect(result.current.isLoading).toEqual(true);
    await waitForNextUpdate();
    expect(result.current.error!.message).toEqual('errr');
    expect(result.current.isLoading).toEqual(true);
    done();
  });
  it('should throw an error if array is empty', async done => {
    try {
      useFetches([]);
      const { result, waitForNextUpdate } = renderHook(useFetches, { initialProps: [] });
      await waitForNextUpdate().then(done);
      expect(result.error.message).toEqual("Doesn't have url to fetch.");
      done();
    } catch (e) {
    } finally {
      done();
    }
  });
});
