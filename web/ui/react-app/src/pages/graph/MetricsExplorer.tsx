import React, { Component } from 'react';
import { Modal, ModalBody, ModalHeader } from 'reactstrap';

interface Props {
  show: boolean;
  updateShow(show: boolean): void;
  metrics: string[];
  insertAtCursor(value: string): void;
}

class MetricsExplorer extends Component<Props> {
  handleMetricClick = (query: string): void => {
    this.props.insertAtCursor(query);
    this.props.updateShow(false);
  };

  toggle = (): void => {
    this.props.updateShow(!this.props.show);
  };

  render(): JSX.Element {
    return (
      <Modal isOpen={this.props.show} toggle={this.toggle} className="metrics-explorer">
        <ModalHeader toggle={this.toggle}>Metrics Explorer</ModalHeader>
        <ModalBody>
          {this.props.metrics.map((metric) => (
            <p key={metric} className="metric" onClick={this.handleMetricClick.bind(this, metric)}>
              {metric}
            </p>
          ))}
        </ModalBody>
      </Modal>
    );
  }
}

export default MetricsExplorer;
