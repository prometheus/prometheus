import { FC, ReactNode } from "react";
import {
  VectorSelector,
  MatrixSelector,
  nodeType,
  LabelMatcher,
  matchType,
} from "../../../promql/ast";
import { escapeString } from "../../../promql/utils";
import { useSuspenseAPIQuery } from "../../../api/api";
import { Card, Text, Divider, List } from "@mantine/core";
import { MetadataResult } from "../../../api/responseTypes/metadata";
import { formatPrometheusDuration } from "../../../lib/formatTime";
import { useSettings } from "../../../state/settingsSlice";

interface SelectorExplainViewProps {
  node: VectorSelector | MatrixSelector;
}

const matchingCriteriaList = (
  name: string,
  matchers: LabelMatcher[]
): ReactNode => {
  return (
    <List fz="sm" my="md" withPadding>
      {name.length > 0 && (
        <List.Item>
          The metric name is{" "}
          <span className="promql-code promql-metric-name">{name}</span>.
        </List.Item>
      )}
      {matchers
        .filter((m) => !(m.name === "__name__"))
        .map((m) => {
          switch (m.type) {
            case matchType.equal:
              return (
                <List.Item>
                  <span className="promql-code promql-label-name">
                    {m.name}
                  </span>
                  <span className="promql-code promql-operator">{m.type}</span>
                  <span className="promql-code promql-string">
                    "{escapeString(m.value)}"
                  </span>
                  : The label{" "}
                  <span className="promql-code promql-label-name">
                    {m.name}
                  </span>{" "}
                  is exactly{" "}
                  <span className="promql-code promql-string">
                    "{escapeString(m.value)}"
                  </span>
                  .
                </List.Item>
              );
            case matchType.notEqual:
              return (
                <List.Item>
                  <span className="promql-code promql-label-name">
                    {m.name}
                  </span>
                  <span className="promql-code promql-operator">{m.type}</span>
                  <span className="promql-code promql-string">
                    "{escapeString(m.value)}"
                  </span>
                  : The label{" "}
                  <span className="promql-code promql-label-name">
                    {m.name}
                  </span>{" "}
                  is not{" "}
                  <span className="promql-code promql-string">
                    "{escapeString(m.value)}"
                  </span>
                  .
                </List.Item>
              );
            case matchType.matchRegexp:
              return (
                <List.Item>
                  <span className="promql-code promql-label-name">
                    {m.name}
                  </span>
                  <span className="promql-code promql-operator">{m.type}</span>
                  <span className="promql-code promql-string">
                    "{escapeString(m.value)}"
                  </span>
                  : The label{" "}
                  <span className="promql-code promql-label-name">
                    {m.name}
                  </span>{" "}
                  matches the regular expression{" "}
                  <span className="promql-code promql-string">
                    "{escapeString(m.value)}"
                  </span>
                  .
                </List.Item>
              );
            case matchType.matchNotRegexp:
              return (
                <List.Item>
                  <span className="promql-code promql-label-name">
                    {m.name}
                  </span>
                  <span className="promql-code promql-operator">{m.type}</span>
                  <span className="promql-code promql-string">
                    "{escapeString(m.value)}"
                  </span>
                  : The label{" "}
                  <span className="promql-code promql-label-name">
                    {m.name}
                  </span>{" "}
                  does not match the regular expression{" "}
                  <span className="promql-code promql-string">
                    "{escapeString(m.value)}"
                  </span>
                  .
                </List.Item>
              );
            default:
              throw new Error("invalid matcher type");
          }
        })}
    </List>
  );
};

const SelectorExplainView: FC<SelectorExplainViewProps> = ({ node }) => {
  const baseMetricName = node.name.replace(/(_count|_sum|_bucket)$/, "");
  const { lookbackDelta } = useSettings();
  const { data: metricMeta } = useSuspenseAPIQuery<MetadataResult>({
    path: `/metadata`,
    params: {
      metric: baseMetricName,
    },
  });

  return (
    <Card withBorder>
      <Text fz="lg" fw={600} mb="md">
        {node.type === nodeType.vectorSelector ? "Instant" : "Range"} vector
        selector
      </Text>
      <Text fz="sm">
        {metricMeta.data === undefined ||
        metricMeta.data[baseMetricName] === undefined ||
        metricMeta.data[baseMetricName].length < 1 ? (
          <>No metric metadata found.</>
        ) : (
          <>
            <strong>Metric help</strong>:{" "}
            {metricMeta.data[baseMetricName][0].help}
            <br />
            <strong>Metric type</strong>:{" "}
            {metricMeta.data[baseMetricName][0].type}
          </>
        )}
      </Text>
      <Divider my="md" />
      <Text fz="sm">
        {node.type === nodeType.vectorSelector ? (
          <>
            This node selects the latest (non-stale) sample value within the
            last <span className="promql-code promql-duration">{lookbackDelta}</span>
          </>
        ) : (
          <>
            This node selects{" "}
            <span className="promql-code promql-duration">
              {formatPrometheusDuration(node.range)}
            </span>{" "}
            of data going backward from the evaluation timestamp
          </>
        )}
        {node.timestamp !== null ? (
          <>
            , evaluated relative to an absolute evaluation timestamp of{" "}
            <span className="promql-number">
              {(node.timestamp / 1000).toFixed(3)}
            </span>
          </>
        ) : node.startOrEnd !== null ? (
          <>, evaluated relative to the {node.startOrEnd} of the query range</>
        ) : (
          <></>
        )}
        {node.offset === 0 ? (
          <></>
        ) : node.offset > 0 ? (
          <>
            , time-shifted{" "}
            <span className="promql-code promql-duration">
              {formatPrometheusDuration(node.offset)}
            </span>{" "}
            into the past,
          </>
        ) : (
          <>
            , time-shifted{" "}
            <span className="promql-code promql-duration">
              {formatPrometheusDuration(-node.offset)}
            </span>{" "}
            into the future,
          </>
        )}{" "}
        for any series that match all of the following criteria:
      </Text>
      {matchingCriteriaList(node.name, node.matchers)}
      <Text fz="sm">
        If a series has no values in the last{" "}
        <span className="promql-code promql-duration">
          {node.type === nodeType.vectorSelector
            ? lookbackDelta
            : formatPrometheusDuration(node.range)}
        </span>
        {node.offset > 0 && (
          <>
            {" "}
            (relative to the time-shifted instant{" "}
            <span className="promql-code promql-duration">
              {formatPrometheusDuration(node.offset)}
            </span>{" "}
            in the past)
          </>
        )}
        , the series will not be returned.
      </Text>
    </Card>
  );
};

export default SelectorExplainView;
