import React, { Component, ChangeEvent } from 'react';
import { Modal, ModalBody, ModalHeader, Input } from 'reactstrap';
import { Fuzzy, FuzzyResult } from '@nexucis/fuzzy';

const fuz = new Fuzzy({ pre: '<strong>', post: '</strong>', shouldSort: true });

interface MetricsExplorerProps {
  show: boolean;
  updateShow(show: boolean): void;
  metrics: string[];
  insertAtCursor(value: string): void;
}

type MetricsExplorerState = {
  searchTerm: string;
};

class MetricsExplorer extends Component<MetricsExplorerProps, MetricsExplorerState> {
  constructor(props: MetricsExplorerProps) {
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
          {this.state.searchTerm.length > 0 &&
            fuz
              .filter(this.state.searchTerm, this.props.metrics)
              .map((result: FuzzyResult) => (
                <p
                  key={result.original}
                  className="metric"
                  onClick={this.handleMetricClick.bind(this, result.original)}
                  dangerouslySetInnerHTML={{ __html: result.rendered }}
                ></p>
              ))}
          {this.state.searchTerm.length === 0 &&
            this.props.metrics.map((metric) => (
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
