import React, { FC } from 'react';
import { RouteComponentProps } from '@reach/router';
import PathPrefixProps from '../../types/PathPrefixProps';
import { useFetch } from '../../hooks/useFetch';
import { LabelsTable } from './LabelsTable';
import { Target, Labels, DroppedTarget } from '../targets/target';

import { withStatusIndicator } from '../../components/withStatusIndicator';
import { mapObjEntries } from '../../utils';

interface ServiceDiscoveryMap {
  scrape: ServiceMap;
  alertManager: ServiceMap;
}
interface ServiceMap {
  activeTargets: Target[];
  droppedTargets: DroppedTarget[];
}

export interface TargetLabels {
  discoveredLabels: Labels;
  labels: Labels;
  isDropped: boolean;
}

export const processSummary = (activeTargets: Target[], droppedTargets: DroppedTarget[]) => {
  const targets: Record<string, { active: number; total: number }> = {};

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
  for (const target of droppedTargets) {
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

export const processTargets = (activeTargets: Target[], droppedTargets: DroppedTarget[]) => {
  const labels: Record<string, TargetLabels[]> = {};

  for (const target of activeTargets) {
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

  for (const target of droppedTargets) {
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

export const ServiceDiscoveryContent: FC<ServiceDiscoveryMap> = ({ scrape, alertManager }) => {
  const targets = processSummary(scrape.activeTargets, scrape.droppedTargets);
  const labels = processTargets(scrape.activeTargets, scrape.droppedTargets);
  const alertManagerTargets = processSummary(alertManager.activeTargets, alertManager.droppedTargets);
  const alertManagerLabels = processTargets(alertManager.activeTargets, alertManager.droppedTargets);

  return (
    <>
      <h2>Scrape Service Discovery</h2>
      <ul>
        {mapObjEntries(targets, ([k, v]) => (
          <li key={k}>
            <a href={'#' + k}>
              {k} ({v.active} / {v.total} active targets)
            </a>
          </li>
        ))}
      </ul>
      <hr />
      {mapObjEntries(labels, ([k, v]) => {
        return <LabelsTable value={v} name={k} key={k} />;
      })}
      <hr />
      <h2>AlertManager Service Discovery</h2>
      <ul>
        {mapObjEntries(alertManagerTargets, ([k, v]) => (
          <li key={k}>
            <a href={'#' + k}>
              {k} ({v.active} / {v.total} active targets)
            </a>
          </li>
        ))}
      </ul>
      <hr />
      {mapObjEntries(alertManagerLabels, ([k, v]) => {
        return <LabelsTable value={v} name={k} key={k} />;
      })}
      <hr />
    </>
  );
};
ServiceDiscoveryContent.displayName = 'ServiceDiscoveryContent';

const ServicesWithStatusIndicator = withStatusIndicator(ServiceDiscoveryContent);

const ServiceDiscovery: FC<RouteComponentProps & PathPrefixProps> = ({ pathPrefix }) => {
  const { response, error, isLoading } = useFetch<ServiceDiscoveryMap>(`${pathPrefix}/api/v1/servicediscovery`);
  return (
    <ServicesWithStatusIndicator
      {...response.data}
      error={error}
      isLoading={isLoading}
      componentTitle="Service Discovery information"
    />
  );
};

export default ServiceDiscovery;
