export const DEFAULT_HEADERS = {
  Accept: 'application/json',
  'Content-Type': 'application/json',
};

export const handleResponse = (response: Response) => {
  const { headers, ok } = response;
  
  if (!ok) {
    throw new Error(response.statusText);
  }
  const contentType = headers.get('content-type');
  const isJSON = contentType && contentType.includes('application/json');

  return isJSON ? response.json() : response.text();
};

export const getOptions = (options: RequestInit): RequestInit => ({
  ...options,
  credentials: options && options.credentials,
  headers: { ...DEFAULT_HEADERS, ...((options && options.headers) || {}) },
});

export const fetchAPI = async (url: string, options: RequestInit = {}) => {
  const response = await fetch(url, getOptions(options));
  return handleResponse(response);
};

export const get = async (
  url: string,
  urlParams?: { [key: string]: string | number | boolean },
  options: RequestInit = {}
) => {
  const urlString = urlParams ? `${url}?${toQueryString(urlParams)}` : url;
  const response = await fetch(urlString, getOptions(options));
  return handleResponse(response);
};

export const toQueryString = (urlParams: { [key: string]: string | number | boolean }) => {
  return Object.entries(urlParams)
    .map(([k, v]) => `${encodeURIComponent(k)}=${encodeURIComponent(v)}`)
    .join('&');
};
