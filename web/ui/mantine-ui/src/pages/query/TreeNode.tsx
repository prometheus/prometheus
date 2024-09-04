import {
  FC,
  useEffect,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import ASTNode, { nodeType } from "../../promql/ast";
import { getNodeChildren } from "../../promql/utils";
import { formatNode } from "../../promql/format";
import { Box, CSSProperties, Group, Loader, Text } from "@mantine/core";
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

const nodeIndent = 20;

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
  parentRef?: React.RefObject<HTMLDivElement>;
  reportNodeState?: (state: NodeState) => void;
  reverse: boolean;
}> = ({
  node,
  selectedNode,
  setSelectedNode,
  parentRef,
  reportNodeState,
  reverse,
}) => {
  const nodeID = useId();
  const nodeRef = useRef<HTMLDivElement>(null);
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
    labelCardinalities: Record<string, number>;
    labelExamples: Record<string, { value: string; count: number }[]>;
  }>({
    numSeries: 0,
    labelCardinalities: {},
    labelExamples: {},
  });

  const children = getNodeChildren(node);

  const [childStates, setChildStates] = useState<NodeState[]>(
    children.map(() => "waiting")
  );
  const mergedChildState = useMemo(
    () => mergeChildStates(childStates),
    [childStates]
  );

  const { data, error, isFetching } = useAPIQuery<InstantQueryResult>({
    key: [useId()],
    path: "/query",
    params: {
      query: serializeNode(node),
    },
    recordResponseTime: setResponseTime,
    enabled: mergedChildState === "success",
  });

  useEffect(() => {
    if (mergedChildState === "error") {
      reportNodeState && reportNodeState("error");
    }
  }, [mergedChildState, reportNodeState]);

  useEffect(() => {
    if (error) {
      reportNodeState && reportNodeState("error");
    }
  }, [error]);

  useEffect(() => {
    if (isFetching) {
      reportNodeState && reportNodeState("running");
    }
  }, [isFetching]);

  // Update the size and position of tree connector lines based on the node's and its parent's position.
  useLayoutEffect(() => {
    if (parentRef === undefined) {
      // We're the root node.
      return;
    }

    if (parentRef.current === null || nodeRef.current === null) {
      return;
    }
    const parentRect = parentRef.current.getBoundingClientRect();
    const nodeRect = nodeRef.current.getBoundingClientRect();
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
  }, [parentRef, reverse, nodeRef]);

  // Update the node info state based on the query result.
  useEffect(() => {
    if (!data) {
      return;
    }

    reportNodeState && reportNodeState("success");

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
          // count_over_time(foo[7d]) optimization since that removes the metric name.
          if (ln !== "__name__") {
            if (!labelValuesByName.hasOwnProperty(ln)) {
              labelValuesByName[ln] = { [lv]: 1 };
            } else {
              if (!labelValuesByName[ln].hasOwnProperty(lv)) {
                labelValuesByName[ln][lv] = 1;
              } else {
                labelValuesByName[ln][lv]++;
              }
            }
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
        .slice(0, 5)
        .map(([lv, cnt]) => ({ value: lv, count: cnt }));
    });

    setResultStats({
      numSeries: resultSeries,
      labelCardinalities,
      labelExamples,
    });
  }, [data]);

  const innerNode = (
    <Group
      w="fit-content"
      gap="sm"
      my="sm"
      wrap="nowrap"
      pos="relative"
      align="center"
    >
      {parentRef && (
        // Connector line between this node and its parent.
        <Box pos="absolute" display="inline-block" style={connectorStyle} />
      )}
      {/* The node itself. */}
      <Box
        ref={nodeRef}
        w="fit-content"
        px={10}
        py={5}
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
          <IconPointFilled size={18} />
        </Group>
      ) : mergedChildState === "running" ? (
        <Loader size={14} color="gray" type="dots" />
      ) : mergedChildState === "error" ? (
        <Group c="orange.7" gap={5} fz="xs" wrap="nowrap">
          <IconPointFilled size={18} /> Blocked on child query error
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
          <IconPointFilled size={18} />
          <Text fz="xs">
            <strong>Error executing query:</strong> {error.message}
          </Text>
        </Group>
      ) : (
        <Text c="dimmed" fz="xs">
          {resultStats.numSeries} result{resultStats.numSeries !== 1 && "s"} –{" "}
          {responseTime}ms
          {/* {resultStats.numSeries > 0 && (
            <>
              {labelNames.length > 0 ? (
                labelNames.map((v, idx) => (
                  <React.Fragment key={idx}>
                    {idx !== 0 && ", "}
                    {v}
                  </React.Fragment>
                ))
              ) : (
                <>no labels</>
              )}
            </>
          )} */}
        </Text>
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
            parentRef={nodeRef}
            reverse={true}
            reportNodeState={(state: NodeState) => {
              setChildStates((prev) => {
                const newStates = [...prev];
                newStates[0] = state;
                return newStates;
              });
            }}
          />
        </Box>
        {innerNode}
        <Box ml={nodeIndent}>
          <TreeNode
            node={children[1]}
            selectedNode={selectedNode}
            setSelectedNode={setSelectedNode}
            parentRef={nodeRef}
            reverse={false}
            reportNodeState={(state: NodeState) => {
              setChildStates((prev) => {
                const newStates = [...prev];
                newStates[1] = state;
                return newStates;
              });
            }}
          />
        </Box>
      </div>
    );
  } else {
    return (
      <div>
        {innerNode}
        {children.map((child, idx) => (
          <Box ml={nodeIndent} key={idx}>
            <TreeNode
              node={child}
              selectedNode={selectedNode}
              setSelectedNode={setSelectedNode}
              parentRef={nodeRef}
              reverse={false}
              reportNodeState={(state: NodeState) => {
                setChildStates((prev) => {
                  const newStates = [...prev];
                  newStates[idx] = state;
                  return newStates;
                });
              }}
            />
          </Box>
        ))}
      </div>
    );
  }
};

export default TreeNode;
