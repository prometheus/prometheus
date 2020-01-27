import React, { FC, CSSProperties, Fragment } from 'react';
import { Rule } from '../../types/types';
import { Link } from '@reach/router';
import { createExpressionLink } from '../../utils';

export const RulePanel: FC<{ rule: Rule; styles?: CSSProperties; tag?: keyof JSX.IntrinsicElements }> = ({
  rule,
  styles = {},
  tag,
}) => {
  const Tag = tag || Fragment;
  const { name, query, labels, annotations, alerts } = rule;
  const mergedStyles = { background: '#f5f5f5', ...styles };
  return (
    <Tag style={mergedStyles}>
      <pre className="m-0" style={tag ? undefined : mergedStyles}>
        <code>
          <div>
            {rule.alerts ? 'name: ' : 'record: '}
            <Link to={createExpressionLink(`ALERTS{alertname="${name}"}`)}>{name}</Link>
          </div>
          <div>
            expr: <Link to={createExpressionLink(query)}>{query}</Link>
          </div>
          {alerts && (
            <>
              <div>
                <div>labels:</div>
                {Object.entries(labels).map(([key, value]) => (
                  <div className="ml-4" key={key}>
                    {key}: {value}
                  </div>
                ))}
              </div>
              <div>
                <div>annotations:</div>
                {Object.entries(annotations).map(([key, value]) => (
                  <div className="ml-4" key={key}>
                    {key}: {value}
                  </div>
                ))}
              </div>
            </>
          )}
        </code>
      </pre>
    </Tag>
  );
};
