import React, { FC, useState } from "react";
import { BinaryExpr, vectorMatchCardinality } from "../../../../promql/ast";
import { InstantSample, Metric } from "../../../../api/responseTypes/query";
import { isComparisonOperator, isSetOperator } from "../../../../promql/utils";
import {
  VectorMatchError,
  BinOpMatchGroup,
  MatchErrorType,
  computeVectorVectorBinOp,
  filteredSampleValue,
  MaybeFilledInstantSample,
} from "../../../../promql/binOp";
import { formatNode, labelNameList } from "../../../../promql/format";
import {
  Alert,
  Anchor,
  Box,
  Group,
  List,
  Switch,
  Table,
  Text,
} from "@mantine/core";
import { useLocalStorage } from "@mantine/hooks";
import { IconAlertTriangle } from "@tabler/icons-react";
import SeriesName from "../../SeriesName";

// We use this color pool for two purposes:
//
// 1. To distinguish different match groups from each other.
// 2. To distinguish multiple series within one match group from each other.
const colorPool = [
  "#1f77b4",
  "#ff7f0e",
  "#2ca02c",
  "#d62728",
  "#9467bd",
  "#8c564b",
  "#e377c2",
  "#7f7f7f",
  "#bcbd22",
  "#17becf",
  "#393b79",
  "#637939",
  "#8c6d31",
  "#843c39",
  "#d6616b",
  "#7b4173",
  "#ce6dbd",
  "#9c9ede",
  "#c5b0d5",
  "#c49c94",
  "#f7b6d2",
  "#c7c7c7",
  "#dbdb8d",
  "#9edae5",
  "#393b79",
  "#637939",
  "#8c6d31",
  "#843c39",
  "#d6616b",
  "#7b4173",
  "#ce6dbd",
  "#9c9ede",
  "#c5b0d5",
  "#c49c94",
  "#f7b6d2",
  "#c7c7c7",
  "#dbdb8d",
  "#9edae5",
  "#17becf",
  "#393b79",
  "#637939",
  "#8c6d31",
  "#843c39",
  "#d6616b",
  "#7b4173",
  "#ce6dbd",
  "#9c9ede",
  "#c5b0d5",
  "#c49c94",
  "#f7b6d2",
];

const rhsColorOffset = colorPool.length / 2 + 3;
const colorForIndex = (idx: number, offset?: number) =>
  `${colorPool[(idx + (offset || 0)) % colorPool.length]}80`;

const seriesSwatch = (color: string) => (
  <Box
    display="inline-block"
    w={12}
    h={12}
    bg={color}
    style={{
      borderRadius: 2,
      flexShrink: 0,
    }}
  />
);
interface VectorVectorBinaryExprExplainViewProps {
  node: BinaryExpr;
  lhs: InstantSample[];
  rhs: InstantSample[];
}

const noMatchLabels = (
  metric: Metric,
  on: boolean,
  labels: string[]
): Metric => {
  const result: Metric = {};
  for (const name in metric) {
    if (!(labels.includes(name) === on && (on || name !== "__name__"))) {
      result[name] = metric[name];
    }
  }
  return result;
};

const explanationText = (node: BinaryExpr): React.ReactNode => {
  const matching = node.matching!;
  const [oneSide, manySide] =
    matching.card === vectorMatchCardinality.oneToMany
      ? ["left", "right"]
      : ["right", "left"];

  return (
    <>
      <Text size="sm">
        {isComparisonOperator(node.op) ? (
          <>
            This node filters the series from the left-hand side based on the
            result of a "
            <span className="promql-code promql-operator">{node.op}</span>"
            comparison with matching series from the right-hand side.
          </>
        ) : (
          <>
            This node calculates the result of applying the "
            <span className="promql-code promql-operator">{node.op}</span>"
            operator between the sample values of matching series from two sets
            of time series.
          </>
        )}
      </Text>
      <List my="md" fz="sm" withPadding>
        {(matching.labels.length > 0 || matching.on) &&
          (matching.on ? (
            <List.Item>
              <span className="promql-code promql-keyword">on</span>(
              {labelNameList(matching.labels)}):{" "}
              {matching.labels.length > 0 ? (
                <>
                  series on both sides are matched on the labels{" "}
                  {labelNameList(matching.labels)}
                </>
              ) : (
                <>
                  all series from one side are matched to all series on the
                  other side.
                </>
              )}
            </List.Item>
          ) : (
            <List.Item>
              <span className="promql-code promql-keyword">ignoring</span>(
              {labelNameList(matching.labels)}): series on both sides are
              matched on all of their labels, except{" "}
              {labelNameList(matching.labels)}.
            </List.Item>
          ))}
        {matching.card === vectorMatchCardinality.oneToOne ? (
          <List.Item>
            One-to-one match. Each series from the left-hand side is allowed to
            match with at most one series on the right-hand side, and vice
            versa.
          </List.Item>
        ) : (
          <List.Item>
            <span className="promql-code promql-keyword">group_{manySide}</span>
            ({labelNameList(matching.include)}) : {matching.card} match. Each
            series from the {oneSide}-hand side is allowed to match with
            multiple series from the {manySide}-hand side.
            {matching.include.length !== 0 && (
              <>
                {" "}
                Any {labelNameList(matching.include)} labels found on the{" "}
                {oneSide}-hand side are propagated into the result, in addition
                to the match group's labels.
              </>
            )}
          </List.Item>
        )}
        {(matching.fillValues.lhs !== null ||
          matching.fillValues.rhs !== null) &&
          (matching.fillValues.lhs === matching.fillValues.rhs ? (
            <List.Item>
              <span className="promql-code promql-keyword">fill</span>(
              <span className="promql-code promql-number">
                {matching.fillValues.lhs}
              </span>
              ) : For series on either side missing a match, fill in the sample
              value{" "}
              <span className="promql-code promql-number">
                {matching.fillValues.lhs}
              </span>
              .
            </List.Item>
          ) : (
            <>
              {matching.fillValues.lhs !== null && (
                <List.Item>
                  <span className="promql-code promql-keyword">fill_left</span>(
                  <span className="promql-code promql-number">
                    {matching.fillValues.lhs}
                  </span>
                  ) : For series on the left-hand side missing a match, fill in
                  the sample value{" "}
                  <span className="promql-code promql-number">
                    {matching.fillValues.lhs}
                  </span>
                  .
                </List.Item>
              )}

              {matching.fillValues.rhs !== null && (
                <List.Item>
                  <span className="promql-code promql-keyword">fill_right</span>
                  (
                  <span className="promql-code promql-number">
                    {matching.fillValues.rhs}
                  </span>
                  ) : For series on the right-hand side missing a match, fill in
                  the sample value{" "}
                  <span className="promql-code promql-number">
                    {matching.fillValues.rhs}
                  </span>
                  .
                </List.Item>
              )}
            </>
          ))}
        {node.bool && (
          <List.Item>
            <span className="promql-code promql-keyword">bool</span>: Instead of
            filtering series based on the outcome of the comparison for matched
            series, keep all series, but return the comparison outcome as a
            boolean <span className="promql-code promql-number">0</span> or{" "}
            <span className="promql-code promql-number">1</span> sample value.
          </List.Item>
        )}
      </List>
    </>
  );
};

const explainError = (
  binOp: BinaryExpr,
  _mg: BinOpMatchGroup,
  err: VectorMatchError
) => {
  const fixes = (
    <>
      <Text size="sm">
        <strong>Possible fixes:</strong>
      </Text>
      <List withPadding my="md" fz="sm">
        {err.type === MatchErrorType.multipleMatchesForOneToOneMatching && (
          <List.Item>
            <Text size="sm">
              <strong>
                Allow {err.dupeSide === "left" ? "many-to-one" : "one-to-many"}{" "}
                matching
              </strong>
              : If you want to allow{" "}
              {err.dupeSide === "left" ? "many-to-one" : "one-to-many"}{" "}
              matching, you need to explicitly request it by adding a{" "}
              <span className="promql-code promql-keyword">
                group_{err.dupeSide}()
              </span>{" "}
              modifier to the operator:
            </Text>
            <Text size="sm" ta="center" my="md">
              {formatNode(
                {
                  ...binOp,
                  matching: {
                    ...(binOp.matching
                      ? binOp.matching
                      : {
                          labels: [],
                          on: false,
                          include: [],
                          fillValues: { lhs: null, rhs: null },
                        }),
                    card:
                      err.dupeSide === "left"
                        ? vectorMatchCardinality.manyToOne
                        : vectorMatchCardinality.oneToMany,
                  },
                },
                true,
                1
              )}
            </Text>
          </List.Item>
        )}
        <List.Item>
          <strong>Update your matching parameters:</strong> Consider including
          more differentiating labels in your matching modifiers (via{" "}
          <span className="promql-code promql-keyword">on()</span> /{" "}
          <span className="promql-code promql-keyword">ignoring()</span>) to
          split multiple series into distinct match groups.
        </List.Item>
        <List.Item>
          <strong>Aggregate the input:</strong> Consider aggregating away the
          extra labels that create multiple series per group before applying the
          binary operation.
        </List.Item>
      </List>
    </>
  );

  switch (err.type) {
    case MatchErrorType.multipleMatchesForOneToOneMatching:
      return (
        <>
          <Text size="sm">
            Binary operators only allow <strong>one-to-one</strong> matching by
            default, but we found{" "}
            <strong>multiple series on the {err.dupeSide} side</strong> for this
            match group.
          </Text>
          {fixes}
        </>
      );
    case MatchErrorType.multipleMatchesOnBothSides:
      return (
        <>
          <Text size="sm">
            We found <strong>multiple series on both sides</strong> for this
            match group. Since <strong>many-to-many matching</strong> is not
            supported, you need to ensure that at least one of the sides only
            yields a single series.
          </Text>
          {fixes}
        </>
      );
    case MatchErrorType.multipleMatchesOnOneSide: {
      const [oneSide, manySide] =
        binOp.matching!.card === vectorMatchCardinality.oneToMany
          ? ["left", "right"]
          : ["right", "left"];
      return (
        <>
          <Text size="sm">
            You requested{" "}
            <strong>
              {oneSide === "right" ? "many-to-one" : "one-to-many"} matching
            </strong>{" "}
            via{" "}
            <span className="promql-code promql-keyword">
              group_{manySide}()
            </span>
            , but we also found{" "}
            <strong>multiple series on the {oneSide} side</strong> of the match
            group. Make sure that the {oneSide} side only contains a single
            series.
          </Text>
          {fixes}
        </>
      );
    }
    default:
      throw new Error("unknown match error");
  }
};

const VectorVectorBinaryExprExplainView: FC<
  VectorVectorBinaryExprExplainViewProps
> = ({ node, lhs, rhs }) => {
  // TODO: Don't use Mantine's local storage as a one-off here. Decide whether we
  // want to keep Redux, and then do it only via one or the other everywhere.
  const [showSampleValues, setShowSampleValues] = useLocalStorage<boolean>({
    key: "queryPage.explain.binaryOperators.showSampleValues",
    defaultValue: false,
  });

  const [maxGroups, setMaxGroups] = useState<number | undefined>(100);
  const [maxSeriesPerGroup, setMaxSeriesPerGroup] = useState<
    number | undefined
  >(100);

  const { matching } = node;
  if (matching === null) {
    // The parent should make sure to only pass in vector-vector binops that have their "matching" field filled out.
    throw new Error("missing matching parameters in vector-to-vector binop");
  }

  const { groups: matchGroups, numGroups } = computeVectorVectorBinOp(
    node.op,
    matching,
    node.bool,
    lhs,
    rhs,
    {
      maxGroups: maxGroups,
      maxSeriesPerGroup: maxSeriesPerGroup,
    }
  );
  const errCount = Object.values(matchGroups).filter((mg) => mg.error).length;

  return (
    <>
      <Text size="sm">{explanationText(node)}</Text>

      <Group my="lg" justify="flex-end" gap="xl">
        <Switch
          label="Show sample values"
          checked={showSampleValues}
          onChange={(event) => setShowSampleValues(event.currentTarget.checked)}
        />
      </Group>

      {numGroups > Object.keys(matchGroups).length && (
        <Alert color="yellow" mb="md" icon={<IconAlertTriangle />}>
          Too many match groups to display, only showing{" "}
          {Object.keys(matchGroups).length} out of {numGroups} groups.
          <br />
          <br />
          <Anchor fz="sm" onClick={() => setMaxGroups(undefined)}>
            Show all groups
          </Anchor>
        </Alert>
      )}

      {errCount > 0 && (
        <Alert color="yellow" mb="md" icon={<IconAlertTriangle />}>
          Found matching issues in {errCount} match group
          {errCount > 1 ? "s" : ""}. See below for per-group error details.
        </Alert>
      )}

      <Table fz="xs" withRowBorders={false}>
        <Table.Tbody>
          {Object.values(matchGroups).map((mg, mgIdx) => {
            const { groupLabels, lhs, lhsCount, rhs, rhsCount, result, error } =
              mg;

            const matchGroupTitleRow = (color: string) => (
              <Table.Tr ta="center">
                <Table.Td colSpan={2} style={{ backgroundColor: `${color}25` }}>
                  <SeriesName labels={groupLabels} format={true} />
                </Table.Td>
              </Table.Tr>
            );

            const matchGroupTable = (
              series: MaybeFilledInstantSample[],
              seriesCount: number,
              color: string,
              colorOffset?: number
            ) => (
              <Box
                style={{
                  borderRadius: 3,
                  border: "2px solid",
                  borderColor:
                    seriesCount === 0
                      ? "light-dark(var(--mantine-color-gray-4), var(--mantine-color-gray-7))"
                      : color,
                }}
              >
                <Table fz="xs" withRowBorders={false} verticalSpacing={5}>
                  <Table.Tbody>
                    {seriesCount === 0 ? (
                      <Table.Tr>
                        <Table.Td
                          ta="center"
                          c="light-dark(var(--mantine-color-gray-7), var(--mantine-color-gray-5))"
                          py="md"
                          fw="bold"
                        >
                          no matching series
                        </Table.Td>
                      </Table.Tr>
                    ) : (
                      <>
                        {matchGroupTitleRow(color)}
                        {series.map((s, sIdx) => {
                          if (s.value === undefined) {
                            // TODO: Figure out how to handle native histograms.
                            throw new Error(
                              "Native histograms are not supported yet"
                            );
                          }

                          return (
                            <Table.Tr key={sIdx}>
                              <Table.Td>
                                <Group wrap="nowrap" gap={7} align="center">
                                  {seriesSwatch(
                                    colorForIndex(sIdx, colorOffset)
                                  )}

                                  <SeriesName
                                    labels={noMatchLabels(
                                      s.metric,
                                      matching.on,
                                      matching.labels
                                    )}
                                    format={true}
                                  />
                                  {s.filled && (
                                    <Text size="sm" c="dimmed">
                                      no match, filling in default value
                                    </Text>
                                  )}
                                </Group>
                              </Table.Td>
                              {showSampleValues && (
                                <Table.Td ta="right">{s.value[1]}</Table.Td>
                              )}
                            </Table.Tr>
                          );
                        })}
                      </>
                    )}
                    {seriesCount > series.length && (
                      <Table.Tr>
                        <Table.Td ta="center" py="md" fw="bold" c="gray.6">
                          {seriesCount - series.length} more series omitted
                          &nbsp;&nbsp;–&nbsp;&nbsp;
                          <Anchor
                            size="xs"
                            onClick={() => setMaxSeriesPerGroup(undefined)}
                          >
                            Show all series
                          </Anchor>
                        </Table.Td>
                      </Table.Tr>
                    )}
                  </Table.Tbody>
                </Table>
              </Box>
            );

            const groupColor = colorPool[mgIdx % colorPool.length];

            const lhsTable = matchGroupTable(lhs, lhsCount, groupColor);
            const rhsTable = matchGroupTable(
              rhs,
              rhsCount,
              groupColor,
              rhsColorOffset
            );

            const resultTable = (
              <Box
                style={{
                  borderRadius: 3,
                  border: `2px solid`,
                  borderColor:
                    result.length === 0 || error !== null
                      ? "light-dark(var(--mantine-color-gray-4), var(--mantine-color-gray-7))"
                      : groupColor,
                }}
              >
                <Table fz="xs" withRowBorders={false} verticalSpacing={5}>
                  <Table.Tbody>
                    {error !== null ? (
                      <Table.Tr>
                        <Table.Td
                          ta="center"
                          c="light-dark(var(--mantine-color-gray-7), var(--mantine-color-gray-5))"
                          py="md"
                          fw="bold"
                        >
                          error, result omitted
                        </Table.Td>
                      </Table.Tr>
                    ) : result.length === 0 ? (
                      <Table.Tr>
                        <Table.Td
                          ta="center"
                          c="light-dark(var(--mantine-color-gray-7), var(--mantine-color-gray-5))"
                          py="md"
                          fw="bold"
                        >
                          dropped
                        </Table.Td>
                      </Table.Tr>
                    ) : error !== null ? (
                      <Table.Tr>
                        <Table.Td
                          ta="center"
                          c="light-dark(var(--mantine-color-gray-7), var(--mantine-color-gray-5))"
                          py="md"
                          fw="bold"
                        >
                          error, result omitted
                        </Table.Td>
                      </Table.Tr>
                    ) : (
                      <>
                        {result.map(({ sample, manySideIdx }, resIdx) => {
                          if (sample.value === undefined) {
                            // TODO: Figure out how to handle native histograms.
                            throw new Error(
                              "Native histograms are not supported yet"
                            );
                          }

                          const filtered =
                            sample.value[1] === filteredSampleValue;
                          const [lIdx, rIdx] =
                            matching.card === vectorMatchCardinality.oneToMany
                              ? [0, manySideIdx]
                              : [manySideIdx, 0];

                          return (
                            <Table.Tr key={resIdx}>
                              <Table.Td
                                style={{ opacity: filtered ? 0.5 : 1 }}
                                title={
                                  filtered
                                    ? "Series has been filtered by comparison operator"
                                    : undefined
                                }
                              >
                                <Group
                                  wrap="nowrap"
                                  gap="xs"
                                  align="flex-start"
                                >
                                  <Group wrap="nowrap" gap={0}>
                                    {seriesSwatch(colorForIndex(lIdx))}
                                    <span style={{ color: "#aaa" }}>–</span>
                                    {seriesSwatch(
                                      colorForIndex(rIdx, rhsColorOffset)
                                    )}
                                  </Group>

                                  <SeriesName
                                    labels={sample.metric}
                                    format={true}
                                  />
                                </Group>
                              </Table.Td>
                              {showSampleValues && (
                                <Table.Td ta="right">
                                  {filtered ? (
                                    <span style={{ color: "grey" }}>
                                      filtered
                                    </span>
                                  ) : (
                                    <span>{sample.value[1]}</span>
                                  )}
                                </Table.Td>
                              )}
                            </Table.Tr>
                          );
                        })}
                      </>
                    )}
                  </Table.Tbody>
                </Table>
              </Box>
            );

            return (
              <React.Fragment key={mgIdx}>
                {mgIdx !== 0 && <tr style={{ height: 30 }}></tr>}
                <Table.Tr>
                  <Table.Td colSpan={5}>
                    {error && (
                      <Alert
                        color="red"
                        mb="md"
                        title="Error in match group below"
                        icon={<IconAlertTriangle />}
                      >
                        {explainError(node, mg, error)}
                      </Alert>
                    )}
                  </Table.Td>
                </Table.Tr>
                <Table.Tr>
                  <Table.Td valign="middle" p={0}>
                    {lhsTable}
                  </Table.Td>
                  <Table.Td
                    ta="center"
                    fw={isSetOperator(node.op) ? "bold" : undefined}
                  >
                    {node.op}
                    {node.bool && " bool"}
                  </Table.Td>
                  <Table.Td valign="middle" p={0}>
                    {rhsTable}
                  </Table.Td>
                  <Table.Td ta="center">=</Table.Td>
                  <Table.Td valign="middle" p={0}>
                    {resultTable}
                  </Table.Td>
                </Table.Tr>
              </React.Fragment>
            );
          })}
        </Table.Tbody>
      </Table>
    </>
  );
};

export default VectorVectorBinaryExprExplainView;
