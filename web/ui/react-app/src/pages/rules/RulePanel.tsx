import React, { FC, CSSProperties, Fragment } from 'react';
import { Rule } from '../../types/types';
import { Link } from '@reach/router';
import { createExpressionLink, mapObjEntries, formatRange } from '../../utils';

interface RulePanelProps {
  rule: Rule;
  styles?: CSSProperties;
  tag?: keyof JSX.IntrinsicElements;
}

export const RulePanel: FC<RulePanelProps> = ({ rule, styles = {}, tag }) => {
  const Tag = tag || Fragment;
  const { name, query, labels } = rule;
  const style = { background: '#f5f5f5', ...styles };
  return (
    <Tag {...(tag ? { style } : {})}>
      <pre className="m-0" style={tag ? undefined : style}>
        <code>
          {rule.type === 'alerting' ? 'alert: ' : 'record: '}
          <Link
            to={createExpressionLink(
              rule.type === 'alerting' ? `ALERTS{alertname="${name.replace(/"/g, '\\"')}"}` : name.replace(/"/g, '\\"')
            )}
          >
            {name}
          </Link>
          <div>
            expr: <Link to={createExpressionLink(query)}>{query}</Link>
          </div>
          {rule.type === 'alerting' && rule.duration > 0 && <div>for: {formatRange(rule.duration)}</div>}
          {labels && (
            <>
              labels:
              {mapObjEntries(labels, ([key, value]) => (
                <div className="ml-4" key={key}>
                  {key}: {value}
                </div>
              ))}
            </>
          )}
          {rule.type === 'alerting' && rule.annotations && (
            <>
              annotations:
              {mapObjEntries(rule.annotations, ([key, value]) => (
                <div className="ml-4" key={key}>
                  {key}: {value}
                </div>
              ))}
            </>
          )}
        </code>
      </pre>
    </Tag>
  );
};
