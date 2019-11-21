export type NullableValue<T> = T | null;

export interface GraphSeries {
  labels: { [key: string]: string };
  color: string;
  data: NullableValue<number>[][]; // [x,y][]
  index: number;
}

export interface GraphMetric {
  __name__: string;
  code: string;
  handler: string;
  instance: string;
  job: string;
}
