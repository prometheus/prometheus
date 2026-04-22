import { FC, ReactNode } from "react";
import { Alert, Text, Anchor, Card, Divider, List } from "@mantine/core";
import ASTNode, { nodeType, DurationNode } from "../../../promql/ast";
// import AggregationExplainView from "./Aggregation";
// import BinaryExprExplainView from "./BinaryExpr/BinaryExpr";
// import SelectorExplainView from "./Selector";
import funcDocs from "../../../promql/functionDocs";
import { escapeString } from "../../../promql/utils";
import { formatPrometheusDuration } from "../../../lib/formatTime";
import { formatDurationNode } from "../../../promql/format";
import classes from "./ExplainView.module.css";
import SelectorExplainView from "./Selector";
import AggregationExplainView from "./Aggregation";
import BinaryExprExplainView from "./BinaryExpr/BinaryExpr";
import { IconInfoCircle } from "@tabler/icons-react";
const collectNotes = (
  node: DurationNode | null,
  notes: ReactNode[],
  seen: Set<string>
): void => {
  if (!node || node.type === "numberLiteral") {
    return;
  }
  if (node.op === "step") {
    if (!seen.has("step")) {
      seen.add("step");
      notes.push(
        <>
          <span className="promql-code promql-keyword">step()</span> resolves to
          the query step (the interval between data points in a range query, or
          the default evaluation step for instant queries).
        </>
      );
    }
    return;
  }
  if (node.op === "range") {
    if (!seen.has("range")) {
      seen.add("range");
      notes.push(
        <>
          <span className="promql-code promql-keyword">range()</span> resolves to
          the query range (the total time window covered by a range query).
        </>
      );
    }
    return;
  }
  if ((node.op === "min_of" || node.op === "max_of") && node.lhs && node.rhs) {
    const key = `${node.op}:${formatDurationNode(node.lhs)}:${formatDurationNode(node.rhs)}`;
    if (!seen.has(key)) {
      seen.add(key);
      notes.push(
        <>
          <span className="promql-code promql-keyword">{node.op}()</span> returns
          the {node.op === "min_of" ? "smaller" : "larger"} of{" "}
          <span className="promql-code">{formatDurationNode(node.lhs)}</span> and{" "}
          <span className="promql-code">{formatDurationNode(node.rhs)}</span>.
        </>
      );
    }
  }
  collectNotes(node.lhs, notes, seen);
  collectNotes(node.rhs, notes, seen);
};

const durationExprNote = (expr: DurationNode): ReactNode => {
  const notes: ReactNode[] = [];
  collectNotes(expr, notes, new Set<string>());
  if (notes.length === 0) {
    return null;
  }
  const exprCode = (
    <span className="promql-code">{formatDurationNode(expr)}</span>
  );
  if (notes.length === 1) {
    return (
      <Text fz="sm" mt="sm">
        In the duration expression {exprCode}, {notes[0]}
      </Text>
    );
  }
  return (
    <Text fz="sm" mt="sm">
      In the duration expression {exprCode}:
      <List mt="xs" withPadding>
        {notes.map((note, i) => (
          <List.Item key={i}>{note}</List.Item>
        ))}
      </List>
    </Text>
  );
};

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
            This node evaluates the passed expression as a subquery over{" "}
            {node.rangeExpr ? (
              <>
                a duration defined by{" "}
                <span className="promql-code">
                  {formatDurationNode(node.rangeExpr)}
                </span>
              </>
            ) : (
              <>
                the last{" "}
                <span className="promql-code">
                  <span className="promql-duration">
                    {formatPrometheusDuration(node.range)}
                  </span>
                </span>
              </>
            )}{" "}
            at a query resolution{" "}
            {node.stepExpr ? (
              <>
                of{" "}
                <span className="promql-code">
                  {formatDurationNode(node.stepExpr)}
                </span>
              </>
            ) : node.step > 0 ? (
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
            {node.offsetExpr ? (
              <>
                , time-shifted{" "}
                <span className="promql-code">
                  {formatDurationNode(node.offsetExpr)}
                </span>
                {node.offset > 0
                  ? " into the past"
                  : node.offset < 0
                    ? " into the future"
                    : ""}
              </>
            ) : node.offset === 0 ? (
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
          {node.rangeExpr && durationExprNote(node.rangeExpr)}
          {node.stepExpr && durationExprNote(node.stepExpr)}
          {node.offsetExpr && durationExprNote(node.offsetExpr)}
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
