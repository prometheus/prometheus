import { FC } from "react";
import ASTNode, { Aggregation, aggregationType } from "../../../promql/ast";
import { labelNameList } from "../../../promql/format";
import { parsePrometheusFloat } from "../../../lib/formatFloatValue";
import { Card, Text } from "@mantine/core";

const describeAggregationType = (
  aggrType: aggregationType,
  param: ASTNode | null
) => {
  switch (aggrType) {
    case "sum":
      return "sums over the sample values of the input series";
    case "min":
      return "takes the minimum of the sample values of the input series";
    case "max":
      return "takes the maximum of the sample values of the input series";
    case "avg":
      return "calculates the average of the sample values of the input series";
    case "stddev":
      return "calculates the population standard deviation of the sample values of the input series";
    case "stdvar":
      return "calculates the population standard variation of the sample values of the input series";
    case "count":
      return "counts the number of input series";
    case "group":
      return "groups the input series by the supplied grouping labels, while setting the sample value to 1";
    case "count_values":
      if (param === null) {
        throw new Error(
          "encountered count_values() node without label parameter"
        );
      }
      if (param.type !== "stringLiteral") {
        throw new Error(
          "encountered count_values() node without string literal label parameter"
        );
      }
      return (
        <>
          outputs one time series for each unique sample value in the input
          series (each counting the number of occurrences of that value and
          indicating the original value in the {labelNameList([param.val])}{" "}
          label)
        </>
      );
    case "bottomk":
      if (param === null || param.type !== "numberLiteral") {
        return (
          <>
            returns the bottom{" "}
            <span className="promql-code promql-number">K</span> series by value
          </>
        );
      }
      return (
        <>
          returns the bottom{" "}
          <span className="promql-code promql-number">{param.val}</span> series
          by value
        </>
      );
    case "topk":
      if (param === null || param.type !== "numberLiteral") {
        return (
          <>
            returns the top <span className="promql-code promql-number">K</span>{" "}
            series by value
          </>
        );
      }
      return (
        <>
          returns the top{" "}
          <span className="promql-code promql-number">{param.val}</span> series
          by value
        </>
      );
    case "quantile":
      if (param === null) {
        throw new Error(
          "encountered quantile() node without quantile parameter"
        );
      }
      if (param.type === "numberLiteral") {
        return `calculates the ${param.val}th quantile (${
          parsePrometheusFloat(param.val) * 100
        }th percentile) over the sample values of the input series`;
      }
      return "calculates a quantile over the sample values of the input series";

    case "limitk":
      if (param === null || param.type !== "numberLiteral") {
        return (
          <>
            limits the output to{" "}
            <span className="promql-code promql-number">K</span> series
          </>
        );
      }
      return (
        <>
          limits the output to{" "}
          <span className="promql-code promql-number">{param.val}</span> series
        </>
      );
    case "limit_ratio":
      if (param === null || param.type !== "numberLiteral") {
        return "limits the output to a ratio of the input series";
      }
      return (
        <>
          limits the output to a ratio of{" "}
          <span className="promql-code promql-number">{param.val}</span> (
          {parsePrometheusFloat(param.val) * 100}%) of the input series
        </>
      );
    default:
      throw new Error(`invalid aggregation type ${aggrType}`);
  }
};

const describeAggregationGrouping = (grouping: string[], without: boolean) => {
  if (without) {
    return (
      <>aggregating away the [{labelNameList(grouping)}] label dimensions</>
    );
  }

  if (grouping.length === 1) {
    return <>grouped by their {labelNameList(grouping)} label dimension</>;
  }

  if (grouping.length > 1) {
    return <>grouped by their [{labelNameList(grouping)}] label dimensions</>;
  }

  return "aggregating away any label dimensions";
};

interface AggregationExplainViewProps {
  node: Aggregation;
}

const AggregationExplainView: FC<AggregationExplainViewProps> = ({ node }) => {
  return (
    <Card withBorder>
      <Text fz="lg" fw={600} mb="md">
        Aggregation
      </Text>
      <Text fz="sm">
        This node {describeAggregationType(node.op, node.param)},{" "}
        {describeAggregationGrouping(node.grouping, node.without)}.
      </Text>
    </Card>
  );
};

export default AggregationExplainView;
