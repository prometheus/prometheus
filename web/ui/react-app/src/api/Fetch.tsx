import { Component } from 'react';
import { fetchAPI } from './FetchAPI';

export interface FetchState<T> {
  data?: T;
  error?: Error;
}

interface FetchProps {
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

export class Fetch extends Component<FetchProps, FetchState<any>> {
  state = {
    data: undefined,
    error: undefined,
  };

  componentDidMount() {
    const { url, urls, options } = this.props;
    if (Boolean(url) && Boolean(urls)) {
      throw new Error('Please provide either url or urls param but not both.');
    }
    if (urls && urls.length) {
      this.handleResponse(() => this.getAll(urls, options));
    } else if (url) {
      this.handleResponse(() => fetchAPI(url, options).then(res => res.data));
    } else {
      throw new Error('URL/s is Missing');
    }
  }

  componentDidUpdate(nextProps: FetchProps) {
    const { url: nextURL, urls: nextURLs, options } = nextProps;
    const { url, urls } = this.props;
    if (nextURLs && !urlsEqual(urls, nextURLs)) {
      this.handleResponse(() => this.getAll(nextURLs, options));
    } else if (nextURL && url !== nextURL) {
      this.handleResponse(() => fetchAPI(nextURL, options).then(res => res.data));
    }
  }

  getAll = async (urls: string[], options?: RequestInit) => {
    return await Promise.all(
      urls
        .map(async url => {
          const result = await fetchAPI(url, options);
          return result.data;
        })
        .filter(Boolean)
    );
  };

  handleResponse = async (resultCb: () => Promise<any>) => {
    try {
      this.setState({ data: await resultCb() });
    } catch (error) {
      this.setState({ error });
    }
  };

  render() {
    return this.props.children(this.state);
  }
}
