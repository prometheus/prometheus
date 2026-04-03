import { FC, useMemo, useState } from "react";
import { useSearchQuery, useSuspenseAPIQuery } from "../../../api/api";
import { MetadataResult } from "../../../api/responseTypes/metadata";
import { SearchMetricNameResult } from "../../../api/responseTypes/search";
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

const fuzPre = '<b style="color: rgb(0, 102, 191)">';
const fuzPost = "</b>";

const fuz = new Fuzzy({
  pre: fuzPre,
  post: fuzPost,
  shouldSort: true,
});

const sanitizeOpts = {
  allowedTags: ["b"],
  allowedAttributes: { b: ["style"] },
};

// CLIENT_SIDE_LIMIT is the maximum number of metric names for which client-side
// fuzzy filtering is used. Above this threshold the backend search API is used.
const CLIENT_SIDE_LIMIT = 10000;

type MetricsExplorerProps = {
  metricNames: string[];
  insertText: (text: string) => void;
  close: () => void;
};

const getSearchMatches = (input: string, expressions: string[]) =>
  fuz.filter(input.replace(/ /g, ""), expressions, {
    pre: fuzPre,
    post: fuzPost,
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

  // Probe the backend once to determine whether the full dataset fits within the
  // client-side limit. When has_more is false the dataset is small enough to
  // filter locally; otherwise the backend search API is used.
  const { data: probeData } = useSearchQuery<SearchMetricNameResult>({
    path: "/search/metric_names",
    params: { limit: String(CLIENT_SIDE_LIMIT), case_sensitive: "false" },
  });
  const useClientSide = probeData === undefined || !probeData.hasMore;

  // Backend search: only enabled when the dataset is too large for client-side
  // and the user has actually typed something.
  const searchQuery = debouncedFilterText.replace(/ /g, "");
  const { data: backendData } = useSearchQuery<SearchMetricNameResult>({
    path: "/search/metric_names",
    params: {
      search: searchQuery,
      fuzz_alg: "subsequence",
      fuzz_threshold: "0",
      case_sensitive: "false",
      limit: "100",
    },
    enabled: !useClientSide && searchQuery !== "",
  });

  const searchMatches = useMemo(() => {
    if (debouncedFilterText === "") {
      return metricNames.map((m) => ({ original: m, rendered: m }));
    }
    if (useClientSide) {
      return getSearchMatches(debouncedFilterText, metricNames);
    }
    // Backend mode: names are already filtered by the server; apply local fuzzy
    // only for character-level highlighting.
    const backendNames = (backendData?.results ?? []).map(
      (r: SearchMetricNameResult) => r.name
    );
    return backendNames.map((name) => {
      const m = fuz.match(searchQuery, name, { pre: fuzPre, post: fuzPost });
      return m ?? { original: name, rendered: name };
    });
  }, [
    debouncedFilterText,
    metricNames,
    useClientSide,
    backendData,
    searchQuery,
  ]);

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
