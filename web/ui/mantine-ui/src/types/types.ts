export interface Histogram {
  count: string;
  sum: string;
  buckets?: [number, string, string, string][];
}
