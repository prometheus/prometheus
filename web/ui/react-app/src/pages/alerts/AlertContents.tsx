import React, { FC, useState, Fragment } from 'react';
import { ButtonGroup, Button, Row, Badge } from 'reactstrap';
import CollapsibleAlertPanel from './CollapsibleAlertPanel';
import Checkbox from '../../Checkbox';

export type RuleState = keyof RuleStatus<any>;

export interface Rule {
  alerts: Alert[];
  annotations: { [k: string]: string };
  duration: number;
  health: string;
  labels: { [k: string]: string };
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
  labels: { [k: string]: string };
  state: RuleState;
  value: string;
  annotations: { [k: string]: string };
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

  const [annotationsVisible, setAnnotationsVisibility] = useState(false);

  const toggleState = (ruleState: RuleState) => {
    setState({
      ...state,
      [ruleState]: !state[ruleState],
    });
  };

  return (
    <>
      <ButtonGroup className="mb-3">
        <Button active={state.inactive} onClick={() => toggleState('inactive')} color="primary">
          Inactive({statsCount.inactive})
        </Button>
        <Button active={state.pending} onClick={() => toggleState('pending')} color="primary">
          Pending({statsCount.pending})
        </Button>
        <Button active={state.firing} onClick={() => toggleState('firing')} color="primary">
          Firing({statsCount.firing})
        </Button>
      </ButtonGroup>
      <Row className="mb-2">
        <Checkbox
          id="show_annotations"
          wrapperStyles={{ margin: '0 0 0 15px', alignSelf: 'center' }}
          defaultChecked={annotationsVisible}
          onClick={() => setAnnotationsVisibility(!annotationsVisible)}
        >
          Show annotations
        </Checkbox>
      </Row>
      <div>
        {groups.map(({ rules, name, file }, i) => {
          return (
            <Fragment key={i}>
              <StatusBadges rules={rules}>
                {file} > {name}
              </StatusBadges>
              {rules.map(
                (rule, j) => {
                  return state[rule.state] && (
                    <CollapsibleAlertPanel key={rule.name + i + j} showAnnotations={annotationsVisible} rule={rule} />
                  )
                }
              )}
            </Fragment>
          );
        })}
      </div>
    </>
  );
};

interface StatusBadgesProps {
  rules: Rule[]
}

const StatusBadges: FC<StatusBadgesProps> = ({ rules, children }) => {
  const statesCounter = rules.reduce<any>((acc, r) => {
    return {
      ...acc,
      [r.state]: acc[r.state] + r.alerts.length
    }
  }, {
    firing: 0,
    pending: 0,
  })
  return (
    <div className="alert-group-info border rounded-sm" style={{ lineHeight: 1.1 }}>
      {isNaN(statesCounter.inactive) && <Badge color="success">inactive</Badge>}
      {statesCounter.pending > 0 && <Badge color="warning">pending({statesCounter.pending})</Badge>}
      {statesCounter.firing > 0 && <Badge color="danger">firing({statesCounter.firing})</Badge>}
      {children}
    </div>
  )
}

AlertsContent.displayName = 'Alerts';

export default AlertsContent;
