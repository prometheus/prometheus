export const parsePrometheusFloat = (str: string): number => {
  switch (str) {
    case "+Inf":
      return Infinity;
    case "-Inf":
      return -Infinity;
    default:
      return parseFloat(str);
  }
};

export const formatPrometheusFloat = (num: number): string => {
  switch (num) {
    case Infinity:
      return "+Inf";
    case -Infinity:
      return "-Inf";
    default:
      return num.toString();
  }
};
