import { FC, useMemo, useState } from "react";
import { useSuspenseAPIQuery } from "../../../api/api";
import { MetadataResult } from "../../../api/responseTypes/metadata";
import {
  ActionIcon,
  CopyButton,
  Group,
  Stack,
  Table,
  TextInput,
} from "@mantine/core";
import React from "react";
import { Fuzzy } from "@nexucis/fuzzy";
import sanitizeHTML from "sanitize-html";
import {
  IconCheck,
  IconCodePlus,
  IconCopy,
  IconZoomCode,
} from "@tabler/icons-react";
import LabelsExplorer from "./LabelsExplorer";
import { useDebouncedValue } from "@mantine/hooks";
import classes from "./MetricsExplorer.module.css";
import CustomInfiniteScroll from "../../../components/CustomInfiniteScroll";

const fuz = new Fuzzy({
  pre: '<b style="color: rgb(0, 102, 191)">',
  post: "</b>",
  shouldSort: true,
});

const sanitizeOpts = {
  allowedTags: ["b"],
  allowedAttributes: { b: ["style"] },
};

type MetricsExplorerProps = {
  metricNames: string[];
  insertText: (text: string) => void;
  close: () => void;
};

const getSearchMatches = (input: string, expressions: string[]) =>
  fuz.filter(input.replace(/ /g, ""), expressions, {
    pre: '<b style="color: rgb(0, 102, 191)">',
    post: "</b>",
  });

const MetricsExplorer: FC<MetricsExplorerProps> = ({
  metricNames,
  insertText,
  close,
}) => {
  // Fetch the alerting rules data.
  const { data } = useSuspenseAPIQuery<MetadataResult>({
    path: `/metadata`,
  });
  const [selectedMetric, setSelectedMetric] = useState<string | null>(null);

  const [filterText, setFilterText] = useState("");
  const [debouncedFilterText] = useDebouncedValue(filterText, 250);

  const searchMatches = useMemo(() => {
    if (debouncedFilterText === "") {
      return metricNames.map((m) => ({ original: m, rendered: m }));
    }
    return getSearchMatches(debouncedFilterText, metricNames);
  }, [debouncedFilterText, metricNames]);

  const getMeta = (m: string) => {
    return (
      // First check if the full metric name has metadata (even if it has one of the
      // histogram/summary suffixes, it may be a metric that is not following naming
      // conventions, see https://github.com/prometheus/prometheus/issues/16907).
      data.data[m] ??
      data.data[m.replace(/(_count|_sum|_bucket|_total)$/, "")] ?? [
        { help: "unknown", type: "unknown", unit: "unknown" },
      ]
    );
  };

  if (selectedMetric !== null) {
    return (
      <LabelsExplorer
        metricName={selectedMetric}
        insertText={(text: string) => {
          insertText(text);
          close();
        }}
        hideLabelsExplorer={() => setSelectedMetric(null)}
      />
    );
  }

  return (
    <Stack>
      <TextInput
        title="Filter by text"
        placeholder="Enter text to filter metric names by..."
        value={filterText}
        onChange={(e: React.ChangeEvent<HTMLInputElement>) =>
          setFilterText(e.target.value)
        }
        autoFocus
      />
      <CustomInfiniteScroll
        allItems={searchMatches}
        child={({ items }) => (
          <Table>
            <Table.Thead>
              <Table.Tr>
                <Table.Th>Metric</Table.Th>
                <Table.Th>Type</Table.Th>
                <Table.Th>Help</Table.Th>
              </Table.Tr>
            </Table.Thead>
            <Table.Tbody>
              {items.map((m) => (
                <Table.Tr key={m.original}>
                  <Table.Td>
                    <Group justify="space-between" wrap="nowrap">
                      {debouncedFilterText === "" ? (
                        m.original
                      ) : (
                        <div
                          dangerouslySetInnerHTML={{
                            __html: sanitizeHTML(m.rendered, sanitizeOpts),
                          }}
                        />
                      )}
                      <Group gap="xs">
                        <ActionIcon
                          size="sm"
                          color="gray"
                          variant="light"
                          title="Explore metric"
                          onClick={() => {
                            setSelectedMetric(m.original);
                          }}
                        >
                          <IconZoomCode
                            style={{ width: "70%", height: "70%" }}
                            stroke={1.5}
                          />
                        </ActionIcon>
                        <ActionIcon
                          size="sm"
                          color="gray"
                          variant="light"
                          title="Insert at cursor and close explorer"
                          onClick={() => {
                            insertText(m.original);
                            close();
                          }}
                        >
                          <IconCodePlus
                            style={{ width: "70%", height: "70%" }}
                            stroke={1.5}
                          />
                        </ActionIcon>
                        <CopyButton value={m.original}>
                          {({ copied, copy }) => (
                            <ActionIcon
                              size="sm"
                              color="gray"
                              variant="light"
                              title="Copy to clipboard"
                              onClick={copy}
                            >
                              {copied ? (
                                <IconCheck
                                  style={{ width: "70%", height: "70%" }}
                                  stroke={1.5}
                                />
                              ) : (
                                <IconCopy
                                  style={{ width: "70%", height: "70%" }}
                                  stroke={1.5}
                                />
                              )}
                            </ActionIcon>
                          )}
                        </CopyButton>
                      </Group>
                    </Group>
                  </Table.Td>
                  <Table.Td px="lg">
                    {getMeta(m.original).map((meta, idx) => (
                      <React.Fragment key={idx}>
                        <span className={classes.typeLabel}>{meta.type}</span>
                        <br />
                      </React.Fragment>
                    ))}
                  </Table.Td>
                  <Table.Td>
                    {getMeta(m.original).map((meta, idx) => (
                      <React.Fragment key={idx}>
                        <span className={classes.helpLabel}>{meta.help}</span>
                        <br />
                      </React.Fragment>
                    ))}
                  </Table.Td>
                </Table.Tr>
              ))}
            </Table.Tbody>
          </Table>
        )}
      />
    </Stack>
  );
};

export default MetricsExplorer;
