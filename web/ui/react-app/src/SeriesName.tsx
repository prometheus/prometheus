import React, { PureComponent } from 'react';

interface SeriesNameProps {
  labels: { [key: string]: string } | null;
  format: boolean;
}

class SeriesName extends PureComponent<SeriesNameProps> {
  renderFormatted(): React.ReactNode {
    const labels = this.props.labels!;

    const labelNodes: React.ReactNode[] = [];
    let first = true;
    for (const label in labels) {
      if (label === '__name__') {
        continue;
      }

      labelNodes.push(
        <span key={label}>
          {!first && ', '}
          <span className="legend-label-name">{label}</span>=<span className="legend-label-value">"{labels[label]}"</span>
        </span>
      );

      if (first) {
        first = false;
      }
    }

    return (
      <>
        <span className="legend-metric-name">{labels.__name__ || ''}</span>
        <span className="legend-label-brace">{'{'}</span>
        {labelNodes}
        <span className="legend-label-brace">{'}'}</span>
      </>
    );
  }

  renderPlain() {
    const labels = this.props.labels!;

    let tsName = (labels.__name__ || '') + '{';
    const labelStrings: string[] = [];
    for (const label in labels) {
      if (label !== '__name__') {
        labelStrings.push(label + '="' + labels[label] + '"');
      }
    }
    tsName += labelStrings.join(', ') + '}';
    return tsName;
  }

  render() {
    if (this.props.labels === null) {
      return 'scalar';
    }

    if (this.props.format) {
      return this.renderFormatted();
    }
    // Return a simple text node. This is much faster to scroll through
    // for longer lists (hundreds of items).
    return this.renderPlain();
  }
}

export default SeriesName;
