import { Component } from 'react';

export interface FetcherState<T> {
  data?: T;
  error?: Error;
}

interface FetcherProps {
  url?: string;
  urls?: string[];
  options?: RequestInit;
  children: (state: any) => any;
}

const urlsEqual = (urls?: string[], nextURLs?: string[]) => {
  if (!urls || !nextURLs) {
    return false;
  }
  return JSON.stringify(urls) === JSON.stringify(nextURLs);
};

export class Fetcher extends Component<FetcherProps, FetcherState<any>> {
  state = {
    data: undefined,
    error: undefined,
  };

  componentDidMount() {
    const { url, urls, options } = this.props;
    if (urls && urls.length) {
      this.handleResponse(async () => await Promise.all(urls.map(this.get(options)).filter(Boolean)));
    } else if (url) {
      this.handleResponse(() => this.get(options)(url));
    } else {
      throw new Error('URL/s is Missing');
    }
  }

  componentDidUpdate(nextProps: FetcherProps) {
    const { url: nextURL, urls: nextURLS, options } = nextProps;
    const { url, urls } = this.props;
    if (nextURLS && !urlsEqual(urls, nextURLS)) {
      this.handleResponse(async () => await Promise.all(nextURLS.map(this.get(options)).filter(Boolean)));
    } else if (nextURL && url !== nextURL) {
      this.handleResponse(() => this.get(options)(nextURL));
    }
  }

  handleResponse = async (cb: () => Promise<any>) => {
    try {
      this.setState({ data: await cb() });
    } catch (error) {
      this.setState({ error });
    }
  };
  get = (options: RequestInit = {}) => async (url: string) => {
    const res = await fetch(url, options);
    if (!res.ok) {
      throw new Error(res.statusText);
    }
    const result = await res.json();
    return result.data;
  };

  render() {
    return this.props.children(this.state);
  }
}
