function metricToSeriesName(labels, formatHTML) {
  var tsName = (labels.__name__ || '') + "{";
  var labelStrings = [];
  for (var label in labels) {
    if (label !== '__name__') {
      labelStrings.push((formatHTML ? '<b>' : '') + label + (formatHTML ? '</b>' : '') + '="' + labels[label] + '"');
    }
  }
  tsName += labelStrings.join(', ') + '}';
  return tsName;
};

export default metricToSeriesName;
