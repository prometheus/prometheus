export type Nullable<T> = T | null;

export interface GraphSeries {
  labels: { [key: string]: string };
  color: string;
  data: Nullable<number>[][]; // [x,y][]
  index: number;
}

export interface GraphMetric {
  __name__: string;
  code: string;
  handler: string;
  instance: string;
  job: string;
}

export interface QueryParams {
  startTime: number;
  endTime: number;
  resolution: number;
}
