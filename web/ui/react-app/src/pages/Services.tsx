import React, { Component } from 'react';
import { RouteComponentProps } from '@reach/router';
import PathPrefixProps from '../PathPrefixProps';

interface ServicesState {
  response: Record<string, any>;
  fetchServicesErrorMessage: string;
  targets: any;
  labels: any;
  showMore: boolean;
}

interface LabelState {
  showMore: boolean;
}

interface LabelProps {
  value: any;
}

class Services extends Component<RouteComponentProps & PathPrefixProps, ServicesState> {
  constructor(props: RouteComponentProps & PathPrefixProps) {
    super(props);

    this.state = {
      response: {},
      fetchServicesErrorMessage: '',
      targets: {},
      labels: {},
      showMore: false,
    };
  }

  componentDidMount() {
    fetch(this.props.pathPrefix + '/api/v1/targets')
      .then(resp => {
        if (resp.ok) {
          return resp.json();
        } else {
          throw new Error('Unexpected response status when fetching services: ' + resp.statusText); // TODO extract error
        }
      })
      .then(json => {
        this.setState({ response: json, targets: this.processTargets(json), labels: this.processLabels(json) });
      })
      .catch(err => {
        this.setState({ fetchServicesErrorMessage: err });
      });
  }

  isHealthy = (health: string) => {
    return health === 'up';
  };

  processTargets = (resp: any) => {
    const activeTargets = resp.data.activeTargets,
      targets: any = Object.create({});

    // Get targets of each type along with the total and active end points
    for (const target of activeTargets) {
      const name = target.scrapePool,
        health = target.health;
      if (!targets[name]) {
        targets[name] = {
          total: 1,
          active: 0,
        };
        if (this.isHealthy(health)) {
          targets[name].active = 1;
        }
      } else {
        targets[name].total++;
        if (this.isHealthy(health)) {
          targets[name].active++;
        }
      }
    }
    return targets;
  };

  getLabels = (target: any) => {
    return {
      discoveredLabels: target.discoveredLabels,
      labels: target.labels,
    };
  };

  processLabels = (resp: any) => {
    const labels = Object.create({}),
      activeTargets = resp.data.activeTargets;

    for (const target of activeTargets) {
      const name = target.scrapePool;
      if (!labels[name]) {
        labels[name] = [];
      }
      labels[name].push(this.getLabels(target));
    }

    return labels;
  };

  render() {
    return (
      <>
        <h2>Service Discovery</h2>
        <ul>
          {Object.keys(this.state.targets).map((val, i) => (
            <li key={Object.keys(this.state.targets)[i]}>
              <a href={'#' + val}>
                {' '}
                {val} ({this.state.targets[val].active} / {this.state.targets[val].total} active targets){' '}
              </a>
            </li>
          ))}
        </ul>
        <hr />
        <div className="outer-layer">
          {Object.keys(this.state.labels).map((val: any, i) => {
            const value = this.state.labels[val];
            return (
              <div id={val} key={Object.keys(this.state.labels)[i]} className="label-component">
                <span className="target-head">
                  {' '}
                  {i + 1}. {val}{' '}
                </span>
                <Labels value={value} />
              </div>
            );
          })}
        </div>
      </>
    );
  }
}

class Labels extends Component<LabelProps, LabelState> {
  constructor(props: LabelProps) {
    super(props);

    this.state = {
      showMore: false,
    };
  }

  toggleMore = () => {
    this.setState({ showMore: !this.state.showMore });
  };

  render() {
    return (
      <>
        {!this.state.showMore ? (
          <button className="btn btn-primary btn-sm" onClick={this.toggleMore}>
            More
          </button>
        ) : (
          <>
            <button className="btn btn-primary btn-sm" onClick={this.toggleMore}>
              Less
            </button>
            <div>
              {Object.keys(this.props.value).map((_, i) => {
                return (
                  <div key={'inner-layer-' + i} className="row inner-layer">
                    <div className="col-md-6">
                      {i === 0 ? <div className="head">Discovered Labels</div> : null}
                      <div key={this.props.value[i].discoveredLabels[i] + '-' + i}>
                        {Object.keys(this.props.value[i].discoveredLabels).map((v, _) => (
                          <div className="label-style" key={this.props.value[i].discoveredLabels[v]}>
                            {' '}
                            {v}={this.props.value[i].discoveredLabels[v]}{' '}
                          </div>
                        ))}
                      </div>
                    </div>
                    <div className="col-md-6">
                      {i === 0 ? <div className="head">Target Labels</div> : null}
                      <div key={this.props.value[i].labels[i] + '-' + i}>
                        {Object.keys(this.props.value[i].labels).map((v, _) => (
                          <div className="label-style" key={this.props.value[i].labels[v]}>
                            {' '}
                            {v}={this.props.value[i].labels[v]}{' '}
                          </div>
                        ))}
                      </div>
                    </div>
                  </div>
                );
              })}
            </div>
          </>
        )}
      </>
    );
  }
}

export default Services;
