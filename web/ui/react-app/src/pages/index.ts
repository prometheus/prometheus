import Agent from './agent/Agent';
import Alerts from './alerts/Alerts';
import Config from './config/Config';
import Flags from './flags/Flags';
import Rules from './rules/Rules';
import ServiceDiscovery from './serviceDiscovery/Services';
import Status from './status/Status';
import Targets from './targets/Targets';
import PanelList from './graph/PanelList';
import TSDBStatus from './tsdbStatus/TSDBStatus';
import { withStartingIndicator } from '../components/withStartingIndicator';

const AgentPage = withStartingIndicator(Agent);
const AlertsPage = withStartingIndicator(Alerts);
const ConfigPage = withStartingIndicator(Config);
const FlagsPage = withStartingIndicator(Flags);
const RulesPage = withStartingIndicator(Rules);
const ServiceDiscoveryPage = withStartingIndicator(ServiceDiscovery);
const StatusPage = withStartingIndicator(Status);
const TSDBStatusPage = withStartingIndicator(TSDBStatus);
const TargetsPage = withStartingIndicator(Targets);
const PanelListPage = withStartingIndicator(PanelList);

// prettier-ignore
export {
  AgentPage,
  AlertsPage,
  ConfigPage,
  FlagsPage,
  RulesPage,
  ServiceDiscoveryPage,
  StatusPage,
  TSDBStatusPage,
  TargetsPage,
  PanelListPage
};
