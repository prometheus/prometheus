export interface SearchMetricNameResult {
  name: string;
  cardinality?: number;
  type?: string;
  help?: string;
  unit?: string;
}

export interface SearchNDJSONResponse<T> {
  results: T[];
  hasMore: boolean;
  warnings: string[];
}
