import React, { FC } from 'react';
import { RouteComponentProps } from '@reach/router';
import PathPrefixProps from '../../types/PathPrefixProps';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import { faSpinner } from '@fortawesome/free-solid-svg-icons';
import { Alert } from 'reactstrap';
import { useFetch } from '../../hooks/useFetch';
import { LabelsTable } from './LabelsTable';
import { Target, Labels, DroppedTarget } from '../targets/target';

// TODO: Deduplicate with https://github.com/prometheus/prometheus/blob/213a8fe89a7308e73f22888a963cbf9375217cd6/web/ui/react-app/src/pages/targets/ScrapePoolList.tsx#L11-L14
interface ServiceMap {
  activeTargets: Target[];
  droppedTargets: DroppedTarget[];
}

export interface TargetLabels {
  discoveredLabels: Labels;
  labels: Labels;
  isDropped: boolean;
}

const Services: FC<RouteComponentProps & PathPrefixProps> = ({ pathPrefix }) => {
  const { response, error } = useFetch<ServiceMap>(`${pathPrefix}/api/v1/targets`);

  const processSummary = (response: ServiceMap) => {
    const targets: any = {};

    // Get targets of each type along with the total and active end points
    for (const target of response.activeTargets) {
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
      const { job: name } = target.discoveredLabels;
      if (!targets[name]) {
        targets[name] = {
          total: 0,
          active: 0,
        };
      }
      targets[name].total++;
    }

    return targets;
  };

  const processTargets = (response: Target[], dropped: DroppedTarget[]) => {
    const labels: Record<string, TargetLabels[]> = {};

    for (const target of response) {
      const name = target.scrapePool;
      if (!labels[name]) {
        labels[name] = [];
      }
      labels[name].push({
        discoveredLabels: target.discoveredLabels,
        labels: target.labels,
        isDropped: false,
      });
    }

    for (const target of dropped) {
      const { job: name } = target.discoveredLabels;
      if (!labels[name]) {
        labels[name] = [];
      }
      labels[name].push({
        discoveredLabels: target.discoveredLabels,
        isDropped: true,
        labels: {},
      });
    }

    return labels;
  };

  if (error) {
    return (
      <Alert color="danger">
        <strong>Error:</strong> Error fetching Service-Discovery: {error.message}
      </Alert>
    );
  } else if (response.data) {
    const targets = processSummary(response.data);
    const labels = processTargets(response.data.activeTargets, response.data.droppedTargets);

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
        {Object.keys(labels).map((val: any, i) => {
          const value = labels[val];
          return <LabelsTable value={value} name={val} key={Object.keys(labels)[i]} />;
        })}
      </>
    );
  }
  return <FontAwesomeIcon icon={faSpinner} spin />;
};

export default Services;
