import { FC, useMemo, useState } from "react";
import {
  LabelMatcher,
  matchType,
  nodeType,
  VectorSelector,
} from "../../../promql/ast";
import {
  Alert,
  Anchor,
  Autocomplete,
  Box,
  Button,
  CopyButton,
  Group,
  List,
  Pill,
  Text,
  SegmentedControl,
  Select,
  Skeleton,
  Stack,
  Table,
  rem,
} from "@mantine/core";
import { escapeString } from "../../../lib/escapeString";
import serializeNode from "../../../promql/serialize";
import { SeriesResult } from "../../../api/responseTypes/series";
import { useAPIQuery } from "../../../api/api";
import { Metric } from "../../../api/responseTypes/query";
import {
  IconAlertTriangle,
  IconArrowLeft,
  IconCheck,
  IconCodePlus,
  IconCopy,
  IconX,
} from "@tabler/icons-react";
import { formatNode } from "../../../promql/format";
import classes from "./LabelsExplorer.module.css";
import { buttonIconStyle } from "../../../styles";

type LabelsExplorerProps = {
  metricName: string;
  insertText: (_text: string) => void;
  hideLabelsExplorer: () => void;
};

const LabelsExplorer: FC<LabelsExplorerProps> = ({
  metricName,
  insertText,
  hideLabelsExplorer,
}) => {
  const [expandedLabels, setExpandedLabels] = useState<string[]>([]);
  const [matchers, setMatchers] = useState<LabelMatcher[]>([]);
  const [newMatcher, setNewMatcher] = useState<LabelMatcher | null>(null);
  const [sortByCard, setSortByCard] = useState<boolean>(true);

  const removeMatcher = (name: string) => {
    setMatchers(matchers.filter((m) => m.name !== name));
  };

  const addMatcher = () => {
    if (newMatcher === null) {
      throw new Error("tried to add null label matcher");
    }

    setMatchers([...matchers, newMatcher]);
    setNewMatcher(null);
  };

  const matcherBadge = (m: LabelMatcher) => (
    <Pill
      key={m.name}
      size="md"
      withRemoveButton
      onRemove={() => {
        removeMatcher(m.name);
      }}
      className={classes.promqlPill}
    >
      <span className="promql-code">
        <span className="promql-label-name">{m.name}</span>
        {m.type}
        <span className="promql-string">"{escapeString(m.value)}"</span>
      </span>
    </Pill>
  );

  const selector: VectorSelector = {
    type: nodeType.vectorSelector,
    name: metricName,
    matchers,
    offset: 0,
    timestamp: null,
    startOrEnd: null,
    anchored: false,
    smoothed: false,
  };

  // Based on the selected pool (if any), load the list of targets.
  const { data, error, isLoading } = useAPIQuery<SeriesResult>({
    path: `/series`,
    params: {
      "match[]": serializeNode(selector),
    },
  });

  // When new series data is loaded, update the corresponding label cardinality and example data.
  const [numSeries, sortedLabelCards, labelExamples] = useMemo(() => {
    const labelCardinalities: Record<string, number> = {};
    const labelExamples: Record<string, { value: string; count: number }[]> =
      {};

    const labelValuesByName: Record<string, Record<string, number>> = {};

    if (data !== undefined) {
      data.data.forEach((series: Metric) => {
        Object.entries(series).forEach(([ln, lv]) => {
          if (ln !== "__name__") {
            if (!(ln in labelValuesByName)) {
              labelValuesByName[ln] = { [lv]: 1 };
            } else {
              if (!(lv in labelValuesByName[ln])) {
                labelValuesByName[ln][lv] = 1;
              } else {
                labelValuesByName[ln][lv]++;
              }
            }
          }
        });
      });

      Object.entries(labelValuesByName).forEach(([ln, lvs]) => {
        labelCardinalities[ln] = Object.keys(lvs).length;
        // labelExamples[ln] = Array.from({ length: Math.min(5, lvs.size) }, (i => () => i.next().value)(lvs.keys()));
        // Sort label values by their number of occurrences within this label name.
        labelExamples[ln] = Object.entries(lvs)
          .sort(([, aCnt], [, bCnt]) => bCnt - aCnt)
          .map(([lv, cnt]) => ({ value: lv, count: cnt }));
      });
    }

    // Sort labels by cardinality if desired, so the labels with the most values are at the top.
    const sortedLabelCards = Object.entries(labelCardinalities).sort((a, b) =>
      sortByCard ? b[1] - a[1] : 0
    );

    return [data?.data.length, sortedLabelCards, labelExamples];
  }, [data, sortByCard]);

  if (error) {
    return (
      <Alert
        color="red"
        title="Error querying series"
        icon={<IconAlertTriangle />}
      >
        <strong>Error:</strong> {error.message}
      </Alert>
    );
  }

  return (
    <Stack fz="sm">
      <Stack style={{ overflow: "auto" }}>
        {/* Selector */}
        <Group align="center" mt="lg" wrap="nowrap">
          <Box w={70} fw={700} style={{ flexShrink: 0 }}>
            Selector:
          </Box>
          <Pill.Group>
            <Pill size="md" className={classes.promqlPill}>
              <span style={{ wordBreak: "break-word", whiteSpace: "pre" }}>
                {formatNode(selector, false)}
              </span>
            </Pill>
          </Pill.Group>
          <Group wrap="nowrap">
            <Button
              variant="light"
              size="xs"
              onClick={() => insertText(serializeNode(selector))}
              leftSection={<IconCodePlus style={buttonIconStyle} />}
              title="Insert selector at cursor and close explorer"
            >
              Insert
            </Button>
            <CopyButton value={serializeNode(selector)}>
              {({ copied, copy }) => (
                <Button
                  variant="light"
                  size="xs"
                  leftSection={
                    copied ? (
                      <IconCheck style={buttonIconStyle} />
                    ) : (
                      <IconCopy style={buttonIconStyle} />
                    )
                  }
                  onClick={copy}
                  title="Copy selector to clipboard"
                >
                  Copy
                </Button>
              )}
            </CopyButton>
          </Group>
        </Group>
        {/* Filters */}
        <Group align="center">
          <Box w={70} fw={700} style={{ flexShrink: 0 }}>
            Filters:
          </Box>

          {matchers.length > 0 ? (
            <Pill.Group>{matchers.map((m) => matcherBadge(m))}</Pill.Group>
          ) : (
            <>No label filters</>
          )}
        </Group>
        {/* Number of series */}
        <Group
          style={{ display: "flex", alignItems: "center", marginBottom: 25 }}
        >
          <Box w={70} fw={700} style={{ flexShrink: 0 }}>
            Results:
          </Box>
          <>{numSeries !== undefined ? `${numSeries} series` : "loading..."}</>
        </Group>
      </Stack>
      {/* Sort order */}
      <Group justify="space-between">
        <Box>
          <Button
            variant="light"
            size="xs"
            onClick={hideLabelsExplorer}
            leftSection={<IconArrowLeft style={buttonIconStyle} />}
          >
            Back to all metrics
          </Button>
        </Box>
        <SegmentedControl
          w="fit-content"
          size="xs"
          value={sortByCard ? "cardinality" : "alphabetic"}
          onChange={(value) => setSortByCard(value === "cardinality")}
          data={[
            { label: "By cardinality", value: "cardinality" },
            { label: "Alphabetic", value: "alphabetic" },
          ]}
        />
      </Group>

      {/* Labels and their values */}
      {isLoading ? (
        <Box mt="lg">
          {Array.from(Array(10), (_, i) => (
            <Skeleton key={i} height={40} mb={15} width="100%" />
          ))}
        </Box>
      ) : (
        <Table fz="sm">
          <Table.Thead>
            <Table.Tr>
              <Table.Th>Label</Table.Th>
              <Table.Th>Values</Table.Th>
            </Table.Tr>
          </Table.Thead>
          <Table.Tbody>
            {sortedLabelCards.map(([ln, card]) => (
              <Table.Tr key={ln}>
                <Table.Td w="50%">
                  <form
                    onSubmit={(e: React.FormEvent) => {
                      // Without this, the page gets reloaded for forms that only have a single input field, see
                      // https://stackoverflow.com/questions/1370021/why-does-forms-with-single-input-field-submit-upon-pressing-enter-key-in-input.
                      e.preventDefault();
                    }}
                  >
                    <Group justify="space-between" align="baseline">
                      <span className="promql-code promql-label-name">
                        {ln}
                      </span>
                      {matchers.some((m) => m.name === ln) ? (
                        matcherBadge(matchers.find((m) => m.name === ln)!)
                      ) : newMatcher?.name === ln ? (
                        <Group wrap="nowrap" gap="xs">
                          <Select
                            size="xs"
                            w={50}
                            style={{ width: "auto" }}
                            value={newMatcher.type}
                            data={Object.values(matchType).map((mt) => ({
                              value: mt,
                              label: mt,
                            }))}
                            onChange={(_value, option) =>
                              setNewMatcher({
                                ...newMatcher,
                                type: option.value as matchType,
                              })
                            }
                          />
                          <Autocomplete
                            value={newMatcher.value}
                            size="xs"
                            placeholder="label value"
                            onChange={(value) =>
                              setNewMatcher({ ...newMatcher, value: value })
                            }
                            data={labelExamples[ln].map((ex) => ex.value)}
                            autoFocus
                          />
                          <Button
                            variant="secondary"
                            size="xs"
                            onClick={() => addMatcher()}
                            style={{ flexShrink: 0 }}
                          >
                            Apply
                          </Button>
                          <Button
                            variant="light"
                            w={40}
                            size="xs"
                            onClick={() => setNewMatcher(null)}
                            title="Cancel"
                            style={{ flexShrink: 0 }}
                          >
                            <IconX
                              style={{ width: rem(18), height: rem(18) }}
                            />
                          </Button>
                        </Group>
                      ) : (
                        <Button
                          variant="light"
                          size="xs"
                          mr="xs"
                          onClick={() =>
                            setNewMatcher({
                              name: ln,
                              type: matchType.equal,
                              value: "",
                            })
                          }
                        >
                          Filter...
                        </Button>
                      )}
                    </Group>
                  </form>
                </Table.Td>
                <Table.Td w="50%">
                  <Text fw={700} fz="sm" my="xs">
                    {card} value{card > 1 && "s"}
                  </Text>
                  <List size="sm" listStyleType="none">
                    {(expandedLabels.includes(ln)
                      ? labelExamples[ln]
                      : labelExamples[ln].slice(0, 5)
                    ).map(({ value, count }) => (
                      <List.Item key={value}>
                        <span
                          className={`${classes.labelValue} promql-code promql-string`}
                          onClick={() => {
                            setMatchers([
                              ...matchers.filter((m) => m.name !== ln),
                              { name: ln, type: matchType.equal, value: value },
                            ]);
                            setNewMatcher(null);
                          }}
                          title="Click to filter by value"
                        >
                          "{escapeString(value)}"
                        </span>{" "}
                        ({count} series)
                      </List.Item>
                    ))}

                    {expandedLabels.includes(ln) ? (
                      <List.Item my="xs">
                        <Anchor
                          size="sm"
                          href="#"
                          onClick={(e) => {
                            e.preventDefault();
                            setExpandedLabels(
                              expandedLabels.filter((l) => l != ln)
                            );
                          }}
                        >
                          Hide full values
                        </Anchor>
                      </List.Item>
                    ) : (
                      labelExamples[ln].length > 5 && (
                        <List.Item my="xs">
                          <Anchor
                            size="sm"
                            href="#"
                            onClick={(e) => {
                              e.preventDefault();
                              setExpandedLabels([...expandedLabels, ln]);
                            }}
                          >
                            Show {labelExamples[ln].length - 5} more values...
                          </Anchor>
                        </List.Item>
                      )
                    )}
                  </List>
                </Table.Td>
              </Table.Tr>
            ))}
          </Table.Tbody>
        </Table>
      )}
    </Stack>
  );
};

export default LabelsExplorer;
