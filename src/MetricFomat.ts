function metricToSeriesName(labels: {[key: string]: string}, formatHTML: boolean): string {
  if (labels === null) {
    return 'scalar';
  }
  let tsName = (labels.__name__ || '') + '{';
  let labelStrings: string[] = [];
  for (let label in labels) {
    if (label !== '__name__') {
      labelStrings.push((formatHTML ? '<b>' : '') + label + (formatHTML ? '</b>' : '') + '="' + labels[label] + '"');
    }
  }
  tsName += labelStrings.join(', ') + '}';
  return tsName;
};

export default metricToSeriesName;
