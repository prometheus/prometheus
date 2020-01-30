import React, { FC, CSSProperties, Fragment } from 'react';
import { Rule } from '../../types/types';
import { Link } from '@reach/router';
import { createExpressionLink, isPresent } from '../../utils';

interface RulePanelProps {
  rule: Rule;
  styles?: CSSProperties;
  tag?: keyof JSX.IntrinsicElements;
}

export const RulePanel: FC<RulePanelProps> = ({ rule, styles = {}, tag }) => {
  const Tag = tag || Fragment;
  const { name, query, labels, annotations, type, duration } = rule;
  const style = { background: '#f5f5f5', ...styles };
  return (
    <Tag {...(tag ? { style } : {})}>
      <pre className="m-0" style={tag ? undefined : style}>
        <code>
          <div>
            {/* Type of the rule is either alerting or recording. */}
            {type === 'alerting' ? 'alert: ' : 'record: '}
            <Link to={createExpressionLink(`ALERTS{alertname="${name.replace(/([/"])/g, "\\$1")}"}`)}>{name}</Link>
          </div>
          <div>
            expr: <Link to={createExpressionLink(query)}>{query}</Link>
          </div>
          {isPresent(duration) && <div>for: {duration}</div>}
          {labels && (
            <>
              <div>labels:</div>
              {Object.entries(labels).map(([key, value]) => (
                <div className="ml-4" key={key}>
                  {key}: {value}
                </div>
              ))}
            </>
          )}
          {annotations && (
            <>
              <div>annotations:</div>
              {Object.entries(annotations).map(([key, value]) => (
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
