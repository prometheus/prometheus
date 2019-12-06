import React, { FC, useState, Fragment } from 'react';
import { Link } from '@reach/router';
import { Alert, Collapse, Card, CardBody, Table, Badge } from 'reactstrap';
import { Rule, RuleStatus } from './AlertContents';

interface ColapProps {
  rule: Rule;
  showAnnotations: boolean;
}

export const alertColors: RuleStatus<string> = {
  firing: 'danger',
  pending: 'warning',
  inactive: 'success',
};

const createExpressionLink = (expr: string) => {
  return `../graph?g0.expr=${encodeURIComponent(expr)}&g0.tab=1&g0.stacked=0&g0.range_input=1h`;
};

const CollapsibleAlertPanel: FC<ColapProps> = ({ rule, showAnnotations }) => {
  const [open, toggle] = useState(false);
  const { name, alerts, state, annotations, labels, query } = rule;
  return (
    <>
      <Alert onClick={() => toggle(!open)} color={alertColors[state]} style={{ cursor: 'pointer' }}>
        <strong>{name}</strong>({`${rule.alerts.length} active`})
      </Alert>
      <Collapse isOpen={open} className="mb-2">
        <Card>
          <CardBody tag="pre" style={{ background: '#f5f5f5' }}>
            <code>
              <div>
                name: <Link to={createExpressionLink(name)}>{name}</Link>
              </div>
              <div>
                expr: <Link to={createExpressionLink(query)}>{query}</Link>
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
              {alerts.map(({ labels, activeAt, value }, i) => {
                return (
                  <Fragment key={i}>
                    <tr>
                      <td>
                        {Object.entries(labels).map(([k, v], j) => {
                          return (
                            <Badge key={j} color="primary" className="mr-1">{k}={v}</Badge>
                          );
                        })}
                      </td>
                      <td>
                        <h5>
                          <Badge color={alertColors[rule.state] + ' text-uppercase'} className="px-3">
                            {rule.state}
                          </Badge>
                        </h5>
                      </td>
                      <td>{activeAt}</td>
                      <td>{value}</td>
                    </tr>
                    {showAnnotations && <Annotations annotations={rule.annotations} />}
                  </Fragment>
                );
              })}
            </tbody>
          </Table>
        </Card>
      </Collapse>
    </>
  );
};

interface AnnotationsProps {
  annotations: { [k: string]: string }
}

export const Annotations: FC<AnnotationsProps> = ({ annotations }) => {
  return (
    <Fragment>
      <tr>
        <td colSpan={4}>
          <h5 className="font-weight-bold">Annotations</h5>
        </td>
      </tr>
      <tr>
        <td colSpan={4}>
          {Object.entries(annotations).map(([k, v], i) => {
            return (
              <div key={i}>
                <strong>{k}</strong>
                <div>{v}</div>
              </div>
            );
          })}
        </td>
      </tr>
    </Fragment>
  )
}

export default CollapsibleAlertPanel;
