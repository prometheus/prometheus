import {
  FC,
  useCallback,
  useEffect,
  useLayoutEffect,
  useMemo,
  useState,
} from "react";
import ASTNode, { nodeType } from "../../promql/ast";
import { escapeString, getNodeChildren } from "../../promql/utils";
import { formatNode } from "../../promql/format";
import {
  Box,
  Code,
  CSSProperties,
  Group,
  List,
  Loader,
  rem,
  Text,
  Tooltip,
} from "@mantine/core";
import { useAPIQuery } from "../../api/api";
import {
  InstantQueryResult,
  InstantSample,
  RangeSamples,
} from "../../api/responseTypes/query";
import serializeNode from "../../promql/serialize";
import { IconPointFilled } from "@tabler/icons-react";
import classes from "./TreeNode.module.css";
import clsx from "clsx";
import { useId } from "@mantine/hooks";
import { functionSignatures } from "../../promql/functionSignatures";

const nodeIndent = 20;
const maxLabelNames = 10;
const maxLabelValues = 10;

const nodeIndicatorIconStyle = { width: rem(18), height: rem(18) };

type NodeState = "waiting" | "running" | "error" | "success";

const mergeChildStates = (states: NodeState[]): NodeState => {
  if (states.includes("error")) {
    return "error";
  }
  if (states.includes("waiting")) {
    return "waiting";
  }
  if (states.includes("running")) {
    return "running";
  }

  return "success";
};

const TreeNode: FC<{
  node: ASTNode;
  selectedNode: { id: string; node: ASTNode } | null;
  setSelectedNode: (Node: { id: string; node: ASTNode } | null) => void;
  parentEl?: HTMLDivElement | null;
  reportNodeState?: (childIdx: number, state: NodeState) => void;
  reverse: boolean;
  // The index of this node in its parent's children.
  childIdx: number;
}> = ({
  node,
  selectedNode,
  setSelectedNode,
  parentEl,
  reportNodeState,
  reverse,
  childIdx,
}) => {
  const nodeID = useId();

  // A normal ref won't work properly here because the ref's `current` property
  // going from `null` to defined won't trigger a re-render of the child
  // component, since it's not a React state update. So we manually have to
  // create a state update using a callback ref. See also
  // https://tkdodo.eu/blog/avoiding-use-effect-with-callback-refs
  const [nodeEl, setNodeEl] = useState<HTMLDivElement | null>(null);
  const nodeRef = useCallback((node: HTMLDivElement) => setNodeEl(node), []);

  const [connectorStyle, setConnectorStyle] = useState<CSSProperties>({
    borderColor:
      "light-dark(var(--mantine-color-gray-4), var(--mantine-color-dark-3))",
    borderLeftStyle: "solid",
    borderLeftWidth: 2,
    width: nodeIndent - 7,
    left: -nodeIndent + 7,
  });
  const [responseTime, setResponseTime] = useState<number>(0);
  const [resultStats, setResultStats] = useState<{
    numSeries: number;
    labelExamples: Record<string, { value: string; count: number }[]>;
    sortedLabelCards: [string, number][];
  }>({
    numSeries: 0,
    labelExamples: {},
    sortedLabelCards: [],
  });

  // Select the node when it is mounted and it is the root of the tree.
  useEffect(() => {
    if (parentEl === undefined) {
      setSelectedNode({ id: nodeID, node: node });
    }
  }, [parentEl, setSelectedNode, nodeID, node]);

  // Deselect node when node is unmounted.
  useEffect(() => {
    return () => {
      setSelectedNode(null);
    };
  }, [setSelectedNode]);

  const children = getNodeChildren(node);

  const [childStates, setChildStates] = useState<NodeState[]>(
    children.map(() => "waiting")
  );
  const mergedChildState = useMemo(
    () => mergeChildStates(childStates),
    [childStates]
  );

  // Optimize range vector selector fetches to give us the info we're looking for
  // more cheaply. E.g. 'foo[7w]' can be expensive to fully fetch, but wrapping it
  // in 'last_over_time(foo[7w])' is cheaper and also gives us all the info we
  // need (number of series and labels).
  // Strip anchored/smoothed modifiers when wrapping, as last_over_time doesn't support them.
  let queryNode = node;
  if (queryNode.type === nodeType.matrixSelector) {
    const matrixNode = { ...queryNode };
    matrixNode.anchored = false;
    matrixNode.smoothed = false;
    queryNode = {
      type: nodeType.call,
      func: functionSignatures["last_over_time"],
      args: [matrixNode],
    };
  }

  const { data, error, isFetching } = useAPIQuery<InstantQueryResult>({
    key: [useId()],
    path: "/query",
    params: {
      query: serializeNode(queryNode),
    },
    recordResponseTime: setResponseTime,
    enabled: mergedChildState === "success",
  });

  useEffect(() => {
    if (mergedChildState === "error" && reportNodeState) {
      reportNodeState(childIdx, "error");
    }
  }, [mergedChildState, reportNodeState, childIdx]);

  useEffect(() => {
    if (error && reportNodeState) {
      reportNodeState(childIdx, "error");
    }
  }, [error, reportNodeState, childIdx]);

  useEffect(() => {
    if (isFetching && reportNodeState) {
      reportNodeState(childIdx, "running");
    }
  }, [isFetching, reportNodeState, childIdx]);

  const childReportNodeState = useCallback(
    (childIdx: number, state: NodeState) => {
      setChildStates((prev) => {
        const newStates = [...prev];
        newStates[childIdx] = state;
        return newStates;
      });
    },
    [setChildStates]
  );

  // Update the size and position of tree connector lines based on the node's and its parent's position.
  useLayoutEffect(() => {
    if (parentEl === undefined) {
      // We're the root node.
      return;
    }

    if (parentEl === null || nodeEl === null) {
      // Either of the two connected nodes hasn't been rendered yet.
      return;
    }

    const parentRect = parentEl.getBoundingClientRect();
    const nodeRect = nodeEl.getBoundingClientRect();
    if (reverse) {
      setConnectorStyle((prevStyle) => ({
        ...prevStyle,
        top: "calc(50% - 1px)",
        bottom: nodeRect.bottom - parentRect.top,
        borderTopLeftRadius: 3,
        borderTopStyle: "solid",
        borderBottomLeftRadius: undefined,
      }));
    } else {
      setConnectorStyle((prevStyle) => ({
        ...prevStyle,
        top: parentRect.bottom - nodeRect.top,
        bottom: "calc(50% - 1px)",
        borderBottomLeftRadius: 3,
        borderBottomStyle: "solid",
        borderTopLeftRadius: undefined,
      }));
    }
  }, [parentEl, nodeEl, reverse, nodeRef, setConnectorStyle]);

  // Update the node info state based on the query result.
  useEffect(() => {
    if (!data) {
      return;
    }

    if (reportNodeState) {
      reportNodeState(childIdx, "success");
    }

    let resultSeries = 0;
    const labelValuesByName: Record<string, Record<string, number>> = {};
    const { resultType, result } = data.data;

    if (resultType === "scalar" || resultType === "string") {
      resultSeries = 1;
    } else if (result && result.length > 0) {
      resultSeries = result.length;
      result.forEach((s: InstantSample | RangeSamples) => {
        Object.entries(s.metric).forEach(([ln, lv]) => {
          // TODO: If we ever want to include __name__ here again, we cannot use the
          // last_over_time(foo[7d]) optimization since that removes the metric name.
          if (ln !== "__name__") {
            if (!labelValuesByName[ln]) {
              labelValuesByName[ln] = {};
            }
            labelValuesByName[ln][lv] = (labelValuesByName[ln][lv] || 0) + 1;
          }
        });
      });
    }

    const labelCardinalities: Record<string, number> = {};
    const labelExamples: Record<string, { value: string; count: number }[]> =
      {};
    Object.entries(labelValuesByName).forEach(([ln, lvs]) => {
      labelCardinalities[ln] = Object.keys(lvs).length;
      // Sort label values by their number of occurrences within this label name.
      labelExamples[ln] = Object.entries(lvs)
        .sort(([, aCnt], [, bCnt]) => bCnt - aCnt)
        .slice(0, maxLabelValues)
        .map(([lv, cnt]) => ({ value: lv, count: cnt }));
    });

    setResultStats({
      numSeries: resultSeries,
      sortedLabelCards: Object.entries(labelCardinalities).sort(
        (a, b) => b[1] - a[1]
      ),
      labelExamples,
    });
  }, [data, reportNodeState, childIdx]);

  const innerNode = (
    <Group
      w="fit-content"
      gap="lg"
      my="sm"
      wrap="nowrap"
      pos="relative"
      align="center"
    >
      {parentEl !== undefined && (
        // Connector line between this node and its parent.
        <Box pos="absolute" display="inline-block" style={connectorStyle} />
      )}
      {/* The node (visible box) itself. */}
      <Box
        ref={nodeRef}
        w="fit-content"
        px={10}
        py={4}
        style={{ borderRadius: 4, flexShrink: 0 }}
        className={clsx(classes.nodeText, {
          [classes.nodeTextError]: error,
          [classes.nodeTextSelected]: selectedNode?.id === nodeID,
        })}
        onClick={() => {
          if (selectedNode?.id === nodeID) {
            setSelectedNode(null);
          } else {
            setSelectedNode({ id: nodeID, node: node });
          }
        }}
      >
        {formatNode(node, false, 1)}
      </Box>
      {mergedChildState === "waiting" ? (
        <Group c="gray">
          <IconPointFilled style={nodeIndicatorIconStyle} />
        </Group>
      ) : mergedChildState === "running" ? (
        <Loader size={14} color="gray" type="dots" />
      ) : mergedChildState === "error" ? (
        <Group c="orange.7" gap={5} fz="xs" wrap="nowrap">
          <IconPointFilled style={nodeIndicatorIconStyle} /> Blocked on child
          query error
        </Group>
      ) : isFetching ? (
        <Loader size={14} color="gray" />
      ) : error ? (
        <Group
          gap={5}
          wrap="nowrap"
          style={{ flexShrink: 0 }}
          className={classes.errorText}
        >
          <IconPointFilled style={nodeIndicatorIconStyle} />
          <Text fz="xs">
            <strong>Error executing query:</strong> {error.message}
          </Text>
        </Group>
      ) : (
        <Group gap={0} wrap="nowrap">
          <Text c="dimmed" fz="xs" style={{ whiteSpace: "nowrap" }}>
            {resultStats.numSeries} result{resultStats.numSeries !== 1 && "s"}
            &nbsp;&nbsp;–&nbsp;&nbsp;
            {responseTime}ms
            {resultStats.sortedLabelCards.length > 0 && (
              <>&nbsp;&nbsp;–&nbsp;&nbsp;</>
            )}
          </Text>
          <Group gap="xs" wrap="nowrap">
            {resultStats.sortedLabelCards
              .slice(0, maxLabelNames)
              .map(([ln, cnt]) => (
                <Tooltip
                  key={ln}
                  position="bottom"
                  withArrow
                  color="dark.6"
                  label={
                    <Box p="xs">
                      <List fz="xs">
                        {resultStats.labelExamples[ln].map(
                          ({ value, count }) => (
                            <List.Item key={value} py={1}>
                              <Code c="red.3" bg="gray.8">
                                {escapeString(value)}
                              </Code>{" "}
                              ({count}
                              x)
                            </List.Item>
                          )
                        )}
                        {cnt > maxLabelValues && <li>...</li>}
                      </List>
                    </Box>
                  }
                >
                  <span style={{ cursor: "pointer", whiteSpace: "nowrap" }}>
                    <Text
                      component="span"
                      fz="xs"
                      className="promql-code promql-label-name"
                      c="light-dark(var(--mantine-color-green-9), var(--mantine-color-green-6))"
                    >
                      {ln}
                    </Text>
                    <Text component="span" fz="xs" c="dimmed">
                      : {cnt}
                    </Text>
                  </span>
                </Tooltip>
              ))}
            {resultStats.sortedLabelCards.length > maxLabelNames ? (
              <Text
                component="span"
                c="dimmed"
                fz="xs"
                style={{ whiteSpace: "nowrap" }}
              >
                ...{resultStats.sortedLabelCards.length - maxLabelNames} more...
              </Text>
            ) : null}
          </Group>
        </Group>
      )}
    </Group>
  );

  if (node.type === nodeType.binaryExpr) {
    return (
      <div>
        <Box ml={nodeIndent}>
          <TreeNode
            node={children[0]}
            selectedNode={selectedNode}
            setSelectedNode={setSelectedNode}
            parentEl={nodeEl}
            reverse={true}
            childIdx={0}
            reportNodeState={childReportNodeState}
          />
        </Box>
        {innerNode}
        <Box ml={nodeIndent}>
          <TreeNode
            node={children[1]}
            selectedNode={selectedNode}
            setSelectedNode={setSelectedNode}
            parentEl={nodeEl}
            reverse={false}
            childIdx={1}
            reportNodeState={childReportNodeState}
          />
        </Box>
      </div>
    );
  }

  return (
    <div>
      {innerNode}
      {children.map((child, idx) => (
        <Box ml={nodeIndent} key={idx}>
          <TreeNode
            node={child}
            selectedNode={selectedNode}
            setSelectedNode={setSelectedNode}
            parentEl={nodeEl}
            reverse={false}
            childIdx={idx}
            reportNodeState={childReportNodeState}
          />
        </Box>
      ))}
    </div>
  );
};

export default TreeNode;
