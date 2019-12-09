import React, { FC, useState, Fragment } from 'react';
import { ButtonGroup, Button, Row, Badge } from 'reactstrap';
import CollapsibleAlertPanel from './CollapsibleAlertPanel';
import Checkbox from '../../Checkbox';
import { isPresent } from '../../utils/func';

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

const AlertsContent: FC<AlertsProps> = ({ groups = [], statsCount }) => {
  const [state, setState] = useState<RuleStatus<boolean>>({
    firing: true,
    pending: true,
    inactive: true,
  });
  const [showAnnotations, setShowAnnotations] = useState(false);

  const toggle = (ruleState: RuleState) => () => {
    setState({
      ...state,
      [ruleState]: !state[ruleState],
    });
  };

  return (
    <>
      <ButtonGroup className="mb-3">
        <Button active={state.inactive} onClick={toggle('inactive')} color="primary">
          Inactive ({statsCount.inactive})
        </Button>
        <Button active={state.pending} onClick={toggle('pending')} color="primary">
          Pending ({statsCount.pending})
        </Button>
        <Button active={state.firing} onClick={toggle('firing')} color="primary">
          Firing ({statsCount.firing})
        </Button>
      </ButtonGroup>
      <Row className="mb-2">
        <Checkbox
          id="show-annotations"
          wrapperStyles={{ margin: '0 0 0 15px', alignSelf: 'center' }}
          onClick={() => setShowAnnotations(!showAnnotations)}
        >
          Show annotations
        </Checkbox>
      </Row>
      {groups.map((group, i) => {
        return (
          <Fragment key={i}>
            <StatusBadges rules={group.rules}>
              {group.file} > {group.name}
            </StatusBadges>
            {group.rules.map((rule, j) => {
              return (
                state[rule.state] && (
                  <CollapsibleAlertPanel key={rule.name + j} showAnnotations={showAnnotations} rule={rule} />
                )
              );
            })}
          </Fragment>
        );
      })}
    </>
  );
};

interface StatusBadgesProps {
  rules: Rule[];
}

const StatusBadges: FC<StatusBadgesProps> = ({ rules, children }) => {
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
    <div className="status-badges border rounded-sm" style={{ lineHeight: 1.1 }}>
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
