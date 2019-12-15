declare namespace jquery.flot {
  // eslint-disable-next-line @typescript-eslint/class-name-casing
  interface plot extends jquery.flot.plot {
    destroy: () => void;
  }
  // eslint-disable-next-line @typescript-eslint/class-name-casing
  interface plotOptions extends jquery.flot.plotOptions {
    tooltip: {
      show?: boolean;
      cssClass?: string;
      content: (
        label: string,
        xval: number,
        yval: number,
        flotItem: jquery.flot.item & {
          series: {
            labels: { [key: string]: string };
            color: string;
            data: (number | null)[][]; // [x,y][]
            index: number;
          };
        }
      ) => string | string;
      xDateFormat?: string;
      yDateFormat?: string;
      monthNames?: string;
      dayNames?: string;
      shifts?: {
        x: number;
        y: number;
      };
      defaultTheme?: boolean;
      lines?: boolean;
      onHover?: () => string;
      $compat?: boolean;
    };
    crosshair: Partial<jquery.flot.axisOptions, 'mode' | 'color'>;
    xaxis: { [K in keyof jquery.flot.axisOptions]: jquery.flot.axisOptions[K] } & {
      showTicks: boolean;
      showMinorTicks: boolean;
      timeBase: 'milliseconds';
    };
    series: { [K in keyof jquery.flot.seriesOptions]: jq.flot.seriesOptions[K] } & {
      stack: boolean;
    };
  }
}

interface Color {
  r: number;
  g: number;
  b: number;
  a: number;
  add: (c: string, d: number) => Color;
  scale: (c: string, f: number) => Color;
  toString: () => string;
  normalize: () => Color;
  clone: () => Color;
}

interface JQueryStatic {
  color: {
    extract: (el: JQuery<HTMLElement>, css?: CSSStyleDeclaration) => Color;
    make: (r?: number, g?: number, b?: number, a?: number) => Color;
    parse: (c: string) => Color;
    scale: () => Color;
  };
}
