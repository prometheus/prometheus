import React, { FC, useState, Fragment } from 'react';
import { Badge } from 'reactstrap';
import CollapsibleAlertPanel from './CollapsibleAlertPanel';
import Checkbox from '../../components/Checkbox';
import { isPresent } from '../../utils';

export type RuleState = keyof RuleStatus<any>;

export interface Rule {
  alerts: Alert[];
  annotations: Record<string, string>;
  duration: number;
  health: string;
  labels: Record<string, string>;
  name: string;
  query: string;
  state: RuleState;
  type: string;
}

export interface RuleStatus<T> {
  firing: T;
  pending: T;
  inactive: T;
}

export interface AlertsProps {
  groups?: RuleGroup[];
  statsCount: RuleStatus<number>;
}

interface Alert {
  labels: Record<string, string>;
  state: RuleState;
  value: string;
  annotations: Record<string, string>;
  activeAt: string;
}

interface RuleGroup {
  name: string;
  file: string;
  rules: Rule[];
  interval: number;
}

const stateColorTuples: Array<[RuleState, 'success' | 'warning' | 'danger']> = [
  ['inactive', 'success'],
  ['pending', 'warning'],
  ['firing', 'danger'],
];

const AlertsContent: FC<AlertsProps> = ({ groups = [], statsCount }) => {
  const [filter, setFilter] = useState<RuleStatus<boolean>>({
    firing: true,
    pending: true,
    inactive: true,
  });
  const [showAnnotations, setShowAnnotations] = useState(false);

  const toggleFilter = (ruleState: RuleState) => () => {
    setFilter({
      ...filter,
      [ruleState]: !filter[ruleState],
    });
  };

  return (
    <>
      <div className="d-flex togglers-wrapper">
        {stateColorTuples.map(([state, color]) => {
          return (
            <Checkbox
              key={state}
              wrapperStyles={{ marginRight: 10 }}
              defaultChecked
              id={`${state}-toggler`}
              onClick={toggleFilter(state)}
            >
              <Badge color={color} className="text-capitalize">
                {state} ({statsCount[state]})
              </Badge>
            </Checkbox>
          );
        })}
        <Checkbox
          wrapperStyles={{ marginLeft: 'auto' }}
          id="show-annotations-toggler"
          onClick={() => setShowAnnotations(!showAnnotations)}
        >
          <span style={{ fontSize: '0.9rem', lineHeight: 1.9 }}>Show annotations</span>
        </Checkbox>
      </div>
      {groups.map((group, i) => {
        const hasFilterOn = group.rules.some(rule => filter[rule.state]);
        return hasFilterOn ? (
          <Fragment key={i}>
            <GroupInfo rules={group.rules}>
              {group.file} > {group.name}
            </GroupInfo>
            {group.rules.map((rule, j) => {
              return (
                filter[rule.state] && (
                  <CollapsibleAlertPanel key={rule.name + j} showAnnotations={showAnnotations} rule={rule} />
                )
              );
            })}
          </Fragment>
        ) : null;
      })}
    </>
  );
};

interface GroupInfoProps {
  rules: Rule[];
}

export const GroupInfo: FC<GroupInfoProps> = ({ rules, children }) => {
  const statesCounter = rules.reduce<any>(
    (acc, r) => {
      return {
        ...acc,
        [r.state]: acc[r.state] + r.alerts.length,
      };
    },
    {
      firing: 0,
      pending: 0,
    }
  );

  return (
    <div className="group-info border rounded-sm" style={{ lineHeight: 1.1 }}>
      {children}
      <div className="badges-wrapper">
        {isPresent(statesCounter.inactive) && <Badge color="success">inactive</Badge>}
        {statesCounter.pending > 0 && <Badge color="warning">pending ({statesCounter.pending})</Badge>}
        {statesCounter.firing > 0 && <Badge color="danger">firing ({statesCounter.firing})</Badge>}
      </div>
    </div>
  );
};

AlertsContent.displayName = 'Alerts';

export default AlertsContent;
