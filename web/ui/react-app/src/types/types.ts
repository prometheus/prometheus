export interface Metric {
  [key: string]: string;
}

export interface QueryParams {
  startTime: number;
  endTime: number;
  resolution: number;
}
