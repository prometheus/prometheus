import React, { FC, Component } from 'react';
import { RouteComponentProps } from '@reach/router';
import PathPrefixProps from '../PathPrefixProps';
import { Alert, Button } from 'reactstrap';
import { useFetch } from '../utils/useFetch';

interface LabelState {
  showMore: boolean;
}

interface LabelProps {
  value: any;
}

interface DiscoveredLabels {
  address: string;
  metrics_path: string;
  scheme: string;
  job: string;
  my: string;
  your: string;
}

interface Labels {
  instance: string;
  job: string;
  my: string;
  your: string;
}

interface ActiveTargets {
  discoveredLabels: DiscoveredLabels[];
  labels: Labels[];
  scrapePool: string;
  scrapeUrl: string;
  lastError: string;
  lastScrape: string;
  lastScrapeDuration: number;
  health: string;
}

interface ServiceMap {
  activeTargets: ActiveTargets[];
  droppedTargets: any[];
}

const Services: FC<RouteComponentProps & PathPrefixProps> = ({ pathPrefix }) => {
  const { response, error } = useFetch<ServiceMap>(`${pathPrefix}/api/v1/targets`);
  const isHealthy = (health: string) => {
    return health === 'up';
  };
  const processTargets = (response: ActiveTargets[]) => {
    const activeTargets = response;
    const targets: any = {};

    // Get targets of each type along with the total and active end points
    for (const target of activeTargets) {
      const { scrapePool: name, health } = target;
      if (!targets[name]) {
        targets[name] = {
          total: 1,
          active: 0,
        };
        if (isHealthy(health)) {
          targets[name].active = 1;
        }
      } else {
        targets[name].total++;
        if (isHealthy(health)) {
          targets[name].active++;
        }
      }
    }
    return targets;
  };

  const processLabels = (response: ActiveTargets[]) => {
    const labels: any = {};
    const activeTargets = response;

    for (const target of activeTargets) {
      const name = target.scrapePool;
      if (!labels[name]) {
        labels[name] = [];
      }
      labels[name].push({
        discoveredLabels: target.discoveredLabels,
        labels: target.labels,
      });
    }

    return labels;
  };
  let targets: any;
  let labels: any;

  if (error) {
    return (
      <Alert color="danger">
        <strong>Error:</strong> Error fetching Service-Discovery: {error.message}
      </Alert>
    );
  } else if (response.data) {
    targets = processTargets(response.data.activeTargets);
    labels = processLabels(response.data.activeTargets);

    return (
      <>
        <h2>Service Discovery</h2>
        <ul>
          {Object.keys(targets).map((val, i) => (
            <li key={i}>
              <a href={'#' + val}>
                {' '}
                {val} ({targets[val].active} / {targets[val].total} active targets){' '}
              </a>
            </li>
          ))}
        </ul>
        <hr />
        <div className="outer-layer">
          {Object.keys(labels).map((val: any, i) => {
            const value = labels[val];
            return (
              <div id={val} key={Object.keys(labels)[i]} className="label-component">
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
  return null;
};

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
        <Button size="sm" color="primary" onClick={this.toggleMore}>
          {!this.state.showMore ? 'More' : 'Less'}
        </Button>
        {this.state.showMore ? (
          <>
            <div>
              {Object.keys(this.props.value).map((_, i) => {
                return (
                  <div key={i} className="row inner-layer">
                    <div className="col-md-6">
                      {i === 0 ? <div className="head">Discovered Labels</div> : null}
                      <div>
                        {Object.keys(this.props.value[i].discoveredLabels).map((v, j) => (
                          <div className="label-style" key={j}>
                            {' '}
                            {v}={this.props.value[i].discoveredLabels[v]}{' '}
                          </div>
                        ))}
                      </div>
                    </div>
                    <div className="col-md-6">
                      {i === 0 ? <div className="head">Target Labels</div> : null}
                      <div>
                        {Object.keys(this.props.value[i].labels).map((v, j) => (
                          <div className="label-style" key={j}>
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
        ) : null}
      </>
    );
  }
}

export default Services;
