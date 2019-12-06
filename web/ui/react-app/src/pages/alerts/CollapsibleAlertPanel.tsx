import React, { FC, useState, Fragment } from 'react';
import { Alert, Collapse, Card, CardBody, Table, Badge } from 'reactstrap';
import { Rule, RuleStatus } from './AlertContents';

interface ColapProps {
  rule: Rule;
  showAnnotations: boolean;
}

const colors: RuleStatus<string> = {
  firing: 'danger',
  pending: 'warning',
  inactive: 'success',
};

const CollapsibleAlertPanel: FC<ColapProps> = ({ rule, showAnnotations }) => {
  const [open, toggle] = useState(false);
  const { name, alerts, state, annotations, labels, query } = rule;
  return (
    <>
      <Alert onClick={() => toggle(!open)} color={colors[state]}>
        <strong>{name}</strong>({`${rule.alerts.length} active`})
      </Alert>
      <Collapse isOpen={open} className="mb-2">
        <Card>
          <CardBody tag="pre" style={{ background: '#f5f5f5' }}>
            <code>
              <div>
                name:{' '}
                <a href="#" onClick={console.log}>
                  {name}
                </a>
              </div>
              <div>
                expr:{' '}
                <a href="#" onClick={console.log}>
                  {query}
                </a>
              </div>
              <div>
                <div>labels:</div>
                <div className="ml-5">severity: {labels.severity}</div>
              </div>
              <div>
                <div>annotations:</div>
                <div className="ml-5">summary: {annotations.summary}</div>
              </div>
            </code>
          </CardBody>
          {alerts.map((alert, i) => {
            return (
              <Fragment key={i}>
                <Table bordered className="mb-0">
                  <thead>
                    <tr>
                      <th>Labels</th>
                      <th>State</th>
                      <th>Active Since</th>
                      <th>Value</th>
                    </tr>
                  </thead>
                  <tbody>
                    <tr>
                      <td>
                        {Object.entries(alert.labels).map(([k, v], j) => {
                          return (
                            <Badge key={j} color="primary" className="mr-1">
                              {k}={v}
                            </Badge>
                          );
                        })}
                      </td>
                      <td>
                        <h5>
                          <Badge color={colors[rule.state] + ' text-uppercase'} className="px-3">
                            {rule.state}
                          </Badge>
                        </h5>
                      </td>
                      <td>{alert.activeAt}</td>
                      <td>{alert.value}</td>
                    </tr>
                  </tbody>
                </Table>
                <Collapse isOpen={showAnnotations}>
                  <Table>
                    <tbody>
                      <tr>
                        <td colSpan={4}>
                          <h5 className="font-weight-bold">Annotations</h5>
                        </td>
                      </tr>
                      <tr>
                        <td colSpan={4}>
                          {Object.entries(alert.annotations).map(([k, v], i) => {
                            return (
                              <div key={i}>
                                <strong>{k}</strong>
                                <div>{v}</div>
                              </div>
                            );
                          })}
                        </td>
                      </tr>
                    </tbody>
                  </Table>
                </Collapse>
              </Fragment>
            );
          })}
        </Card>
      </Collapse>
    </>
  );
};

export default CollapsibleAlertPanel;
