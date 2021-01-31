import React, { Component } from 'react'
import { Modal, ModalBody, ModalHeader } from 'reactstrap';
import './ExpressionExplorer.css'

interface Props {
  show: boolean;
  updateShow(show: boolean): void;
  autocompleteSections: { [key: string]: string[] };
  insertAtCursor(value: string): void;
}

interface State { }

class MetricsExplorer extends Component<Props, State> {
  handleMetricClick = (query: string) => {
    this.props.insertAtCursor(query);
    this.props.updateShow(false);
  }

  toggle = () => {
    this.props.updateShow(!this.props.show)
  }

  render() {
    return (
      <Modal isOpen={this.props.show} toggle={this.toggle} className="expression-explorer">
        <ModalHeader toggle={this.toggle}>Expression Explorer</ModalHeader>
        <ModalBody>
          {this.props.autocompleteSections['Metric Names'].map(query => (
            <p className="metric" onClick={this.handleMetricClick.bind(this, query)}>{query}</p>
          ))}
        </ModalBody>
      </Modal>
    );
  }
}

export default MetricsExplorer
