import React, { FC, useState, Fragment } from 'react';
import { Link } from 'react-router-dom';
import { Alert, Collapse, Table, Badge } from 'reactstrap';
import { RuleStatus } from './AlertContents';
import { Rule } from '../../types/types';
import { faChevronDown, faChevronRight } from '@fortawesome/free-solid-svg-icons';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import { createExpressionLink, parsePrometheusFloat, formatDuration } from '../../utils/index';

interface CollapsibleAlertPanelProps {
  rule: Rule;
  showAnnotations: boolean;
}

const alertColors: RuleStatus<string> = {
  firing: 'danger',
  pending: 'warning',
  inactive: 'success',
};

const CollapsibleAlertPanel: FC<CollapsibleAlertPanelProps> = ({ rule, showAnnotations }) => {
  const [open, toggle] = useState(false);

  return (
    <>
      <Alert fade={false} onClick={() => toggle(!open)} color={alertColors[rule.state]} style={{ cursor: 'pointer' }}>
        <FontAwesomeIcon icon={open ? faChevronDown : faChevronRight} fixedWidth />
        <strong>{rule.name}</strong> ({`${rule.alerts.length} active`})
      </Alert>
      <Collapse isOpen={open} className="mb-2">
        {open && (
          <>
            <pre className="alert-cell">
              <code>
                <div>
                  name: <Link to={createExpressionLink(`ALERTS{alertname="${rule.name}"}`)}>{rule.name}</Link>
                </div>
                <div>
                  expr: <Link to={createExpressionLink(rule.query)}>{rule.query}</Link>
                </div>
                {rule.duration > 0 && (
                  <div>
                    <div>for: {formatDuration(rule.duration * 1000)}</div>
                  </div>
                )}
                {rule.keepFiringFor > 0 && (
                  <div>
                    <div>keep_firing_for: {formatDuration(rule.keepFiringFor * 1000)}</div>
                  </div>
                )}
                {rule.labels && Object.keys(rule.labels).length > 0 && (
                  <div>
                    <div>labels:</div>
                    {Object.entries(rule.labels).map(([key, value]) => (
                      <div className="ml-4" key={key}>
                        {key}: {value}
                      </div>
                    ))}
                  </div>
                )}
                {rule.annotations && Object.keys(rule.annotations).length > 0 && (
                  <div>
                    <div>annotations:</div>
                    {Object.entries(rule.annotations).map(([key, value]) => (
                      <div className="ml-4" key={key}>
                        {key}: {value}
                      </div>
                    ))}
                  </div>
                )}
              </code>
            </pre>
            {rule.alerts.length > 0 && (
              <Table bordered size="sm">
                <thead>
                  <tr>
                    <th>Labels</th>
                    <th>State</th>
                    <th>Active Since</th>
                    <th>Value</th>
                  </tr>
                </thead>
                <tbody>
                  {rule.alerts.map((alert, i) => {
                    return (
                      <Fragment key={i}>
                        <tr>
                          <td style={{ verticalAlign: 'middle' }}>
                            {Object.entries(alert.labels).map(([k, v], j) => {
                              return (
                                <Badge key={j} color="primary" className="mr-1">
                                  {k}={v}
                                </Badge>
                              );
                            })}
                          </td>
                          <td>
                            <h5 className="m-0">
                              <Badge color={alertColors[alert.state] + ' text-uppercase'} className="px-3 mr-1">
                                {alert.state}
                              </Badge>
                              {alert.keepFiringSince && (
                                <Badge color="secondary" className="px-3">
                                  Stabilizing
                                </Badge>
                              )}
                            </h5>
                          </td>
                          <td>{alert.activeAt}</td>
                          <td>{parsePrometheusFloat(alert.value)}</td>
                        </tr>
                        {showAnnotations && <Annotations annotations={alert.annotations} />}
                      </Fragment>
                    );
                  })}
                </tbody>
              </Table>
            )}
          </>
        )}
      </Collapse>
    </>
  );
};

interface AnnotationsProps {
  annotations: Record<string, string>;
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
  );
};

export default CollapsibleAlertPanel;
