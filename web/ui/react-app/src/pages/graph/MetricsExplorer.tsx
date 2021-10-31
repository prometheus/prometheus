import React, { Component, ChangeEvent } from 'react';
import { Modal, ModalBody, ModalHeader, Input } from 'reactstrap';

interface Props {
  show: boolean;
  updateShow(show: boolean): void;
  metrics: string[];
  insertAtCursor(value: string): void;
}
type MetricsExplorerState = {
  searchTerm: string;
};
class MetricsExplorer extends Component<Props, MetricsExplorerState> {
  constructor(props: Props) {
    super(props);
    this.state = { searchTerm: '' };
  }
  handleSearchTerm = (event: ChangeEvent<HTMLInputElement>): void => {
    this.setState({ searchTerm: event.target.value });
  };
  handleMetricClick = (query: string): void => {
    this.props.insertAtCursor(query);
    this.props.updateShow(false);
    this.setState({ searchTerm: '' });
  };

  toggle = (): void => {
    this.props.updateShow(!this.props.show);
  };
  render(): JSX.Element {
    return (
      <Modal isOpen={this.props.show} toggle={this.toggle} className="metrics-explorer" scrollable>
        <ModalHeader toggle={this.toggle}>Metrics Explorer</ModalHeader>
        <ModalBody>
          <Input placeholder="Search" value={this.state.searchTerm} type="text" onChange={this.handleSearchTerm} />
          {this.props.metrics
            .filter((metric) => {
              if (this.state.searchTerm === '') return true;
              return metric.toLowerCase().includes(this.state.searchTerm.toLowerCase());
            })
            .map((metric) => (
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
