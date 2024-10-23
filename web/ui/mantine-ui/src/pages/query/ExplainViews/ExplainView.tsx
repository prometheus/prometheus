import { FC } from "react";
import { Alert, Text, Anchor, Card, Divider } from "@mantine/core";
import ASTNode, { nodeType } from "../../../promql/ast";
// import AggregationExplainView from "./Aggregation";
// import BinaryExprExplainView from "./BinaryExpr/BinaryExpr";
// import SelectorExplainView from "./Selector";
import funcDocs from "../../../promql/functionDocs";
import { escapeString } from "../../../promql/utils";
import { formatPrometheusDuration } from "../../../lib/formatTime";
import classes from "./ExplainView.module.css";
import SelectorExplainView from "./Selector";
import AggregationExplainView from "./Aggregation";
import BinaryExprExplainView from "./BinaryExpr/BinaryExpr";
import { IconInfoCircle } from "@tabler/icons-react";
interface ExplainViewProps {
  node: ASTNode | null;
  treeShown: boolean;
  showTree: () => void;
}

const ExplainView: FC<ExplainViewProps> = ({
  node,
  treeShown,
  showTree: setShowTree,
}) => {
  if (node === null) {
    return (
      <Alert title="How to use the Explain view" icon={<IconInfoCircle />}>
        This tab can help you understand the behavior of individual components
        of a query.
        <br />
        <br />
        To use the Explain view,{" "}
        {!treeShown && (
          <>
            <Anchor fz="unset" onClick={setShowTree}>
              enable the query tree view
            </Anchor>{" "}
            (also available via the expression input menu) and then
          </>
        )}{" "}
        select a node in the tree above.
      </Alert>
    );
  }

  switch (node.type) {
    case nodeType.aggregation:
      return <AggregationExplainView node={node} />;
    case nodeType.binaryExpr:
      return <BinaryExprExplainView node={node} />;
    case nodeType.call:
      return (
        <Card withBorder>
          <Text fz="lg" fw={600} mb="md">
            Function call
          </Text>
          <Text fz="sm">
            This node calls the{" "}
            <Anchor
              fz="inherit"
              href={`https://prometheus.io/docs/prometheus/latest/querying/functions/#${node.func.name}`}
              target="_blank"
            >
              <span className="promql-code promql-keyword">
                {node.func.name}()
              </span>
            </Anchor>{" "}
            function{node.args.length > 0 ? " on the provided inputs" : ""}.
          </Text>
          <Divider my="md" />
          {/* TODO: Some docs, like x_over_time, have relative links pointing back to the Prometheus docs,
          make sure to modify those links in the docs extraction so they work from the explain view */}
          <Text fz="sm" className={classes.funcDoc}>
            {funcDocs[node.func.name]}
          </Text>
        </Card>
      );
    case nodeType.matrixSelector:
      return <SelectorExplainView node={node} />;
    case nodeType.subquery:
      return (
        <Card withBorder>
          <Text fz="lg" fw={600} mb="md">
            Subquery
          </Text>
          <Text fz="sm">
            This node evaluates the passed expression as a subquery over the
            last{" "}
            <span className="promql-code promql-duration">
              {formatPrometheusDuration(node.range)}
            </span>{" "}
            at a query resolution{" "}
            {node.step > 0 ? (
              <>
                of{" "}
                <span className="promql-code promql-duration">
                  {formatPrometheusDuration(node.step)}
                </span>
              </>
            ) : (
              "equal to the default rule evaluation interval"
            )}
            {node.timestamp !== null ? (
              <>
                , evaluated relative to an absolute evaluation timestamp of{" "}
                <span className="promql-number">
                  {(node.timestamp / 1000).toFixed(3)}
                </span>
              </>
            ) : node.startOrEnd !== null ? (
              <>
                , evaluated relative to the {node.startOrEnd} of the query range
              </>
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
                into the past
              </>
            ) : (
              <>
                , time-shifted{" "}
                <span className="promql-code promql-duration">
                  {formatPrometheusDuration(-node.offset)}
                </span>{" "}
                into the future
              </>
            )}
            .
          </Text>
        </Card>
      );
    case nodeType.numberLiteral:
      return (
        <Card withBorder>
          <Text fz="lg" fw={600} mb="md">
            Number literal
          </Text>
          <Text fz="sm">
            A scalar number literal with the value{" "}
            <span className="promql-code promql-number">{node.val}</span>.
          </Text>
        </Card>
      );
    case nodeType.parenExpr:
      return (
        <Card withBorder>
          <Text fz="lg" fw={600} mb="md">
            Parentheses
          </Text>
          <Text fz="sm">
            Parentheses that contain a sub-expression to be evaluated.
          </Text>
        </Card>
      );
    case nodeType.stringLiteral:
      return (
        <Card withBorder>
          <Text fz="lg" fw={600} mb="md">
            String literal
          </Text>
          <Text fz="sm">
            A string literal with the value{" "}
            <span className="promql-code promql-string">
              "{escapeString(node.val)}"
            </span>
            .
          </Text>
        </Card>
      );
    case nodeType.unaryExpr:
      return (
        <Card withBorder>
          <Text fz="lg" fw={600} mb="md">
            Unary expression
          </Text>
          <Text fz="sm">
            A unary expression that{" "}
            {node.op === "+"
              ? "does not affect the expression it is applied to"
              : "changes the sign of the expression it is applied to"}
            .
          </Text>
        </Card>
      );
    case nodeType.vectorSelector:
      return <SelectorExplainView node={node} />;
    default:
      throw new Error("invalid node type");
  }
};

export default ExplainView;
