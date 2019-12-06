import React, { FC, useState, Fragment } from 'react';
import { ButtonGroup, Button, Row, Badge } from 'reactstrap';
import CollapsibleAlertPanel, { alertColors } from './CollapsibleAlertPanel';
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
    firing: false,
    pending: false,
    inactive: false,
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
        <Button onClick={() => toggleState('inactive')} color="primary">
          Inactive({statsCount.inactive})
        </Button>
        <Button onClick={() => toggleState('pending')} color="primary">
          Pending({statsCount.pending})
        </Button>
        <Button onClick={() => toggleState('firing')} color="primary">
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
        {groups.map((g, i) => {
          return (
            <Fragment key={i}>
              <div className="d-flex mb-2 border p-3 rounded-sm" style={{ lineHeight: 0.9 }}>
                {g.rules.map(r => {
                  return (
                    <Badge className="mr-1" color={alertColors[r.state]}>
                      <span>{r.state}</span>
                    </Badge>
                  );
                })}
                {g.file} > {g.name}
              </div>
              {g.rules.map(
                (rule, j) =>
                  state[rule.state] && (
                    <CollapsibleAlertPanel key={rule.name + i + j} showAnnotations={annotationsVisible} rule={rule} />
                  )
              )}
            </Fragment>
          );
        })}
      </div>
    </>
  );
};

AlertsContent.displayName = 'Alerts';

export default AlertsContent;
