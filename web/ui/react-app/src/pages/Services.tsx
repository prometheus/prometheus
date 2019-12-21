import React, { FC } from 'react';
import { RouteComponentProps } from '@reach/router';
import PathPrefixProps from '../PathPrefixProps';
import { Alert } from 'reactstrap';
import { useFetch } from '../utils/useFetch';
import { LabelsTable } from './LabelsTable';

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
  const processTargets = (response: ServiceMap) => {
    const activeTargets = response.activeTargets;
    const targets: any = {};

    // Get targets of each type along with the total and active end points
    for (const target of activeTargets) {
      const { scrapePool: name } = target;
      if (!targets[name]) {
        targets[name] = {
          total: 0,
          active: 0,
        };
      }
      targets[name].total++;
      targets[name].active++;
    }
    for (const target of response.droppedTargets) {
      const { scrapePool: name } = target;
      targets[name].total++;
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
    targets = processTargets(response.data);
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
                <LabelsTable value={value} />
              </div>
            );
          })}
        </div>
      </>
    );
  }
  return null;
};

export default Services;
