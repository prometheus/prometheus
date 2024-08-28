import { FC, useState } from "react";
import { useSuspenseAPIQuery } from "../../../api/api";
import { MetadataResult } from "../../../api/responseTypes/metadata";
import {
  ActionIcon,
  Alert,
  Anchor,
  Group,
  Stack,
  Table,
  TextInput,
} from "@mantine/core";
import React from "react";
import { Fuzzy } from "@nexucis/fuzzy";
import sanitizeHTML from "sanitize-html";
import { IconCopy, IconTerminal, IconZoomIn } from "@tabler/icons-react";

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
  // const metricMeta = promAPI.useFetchAPI<MetricMetadata>(`/api/v1/metadata`);
  // Fetch the alerting rules data.
  const { data } = useSuspenseAPIQuery<MetadataResult>({
    path: `/metadata`,
  });
  const [selectedMetric, setSelectedMetric] = useState<string | null>(null);

  const [filterText, setFilterText] = useState<string>("");

  const getMeta = (m: string) =>
    data.data[m.replace(/(_count|_sum|_bucket)$/, "")] || [
      { help: "unknown", type: "unknown", unit: "unknown" },
    ];

  if (selectedMetric !== null) {
    return (
      <Alert>
        TODO: The labels explorer for a metric still needs to be implemented.
        <br />
        <br />
        <Anchor fz="1em" onClick={() => setSelectedMetric(null)}>
          Back to metrics list
        </Anchor>
      </Alert>
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
      <Table>
        <Table.Thead>
          <Table.Tr>
            <Table.Th>Metric</Table.Th>
            <Table.Th>Type</Table.Th>
            <Table.Th>Help</Table.Th>
          </Table.Tr>
        </Table.Thead>
        <Table.Tbody>
          {(filterText === ""
            ? metricNames.map((m) => ({ original: m, rendered: m }))
            : getSearchMatches(filterText, metricNames)
          ).map((m) => (
            <Table.Tr key={m.original}>
              <Table.Td>
                <Group justify="space-between">
                  <div
                    dangerouslySetInnerHTML={{
                      __html: sanitizeHTML(m.rendered, sanitizeOpts),
                    }}
                  />
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
                      <IconZoomIn
                        style={{ width: "70%", height: "70%" }}
                        stroke={1.5}
                      />
                    </ActionIcon>
                    <ActionIcon
                      size="sm"
                      color="gray"
                      variant="light"
                      title="Insert at cursor"
                      onClick={() => {
                        insertText(m.original);
                        close();
                      }}
                    >
                      <IconTerminal
                        style={{ width: "70%", height: "70%" }}
                        stroke={1.5}
                      />
                    </ActionIcon>
                    <ActionIcon
                      size="sm"
                      color="gray"
                      variant="light"
                      title="Copy to clipboard"
                      onClick={() => {
                        navigator.clipboard.writeText(m.original);
                      }}
                    >
                      <IconCopy
                        style={{ width: "70%", height: "70%" }}
                        stroke={1.5}
                      />
                    </ActionIcon>
                  </Group>
                </Group>
              </Table.Td>
              <Table.Td c="cyan.9" fs="italic" px="lg">
                {getMeta(m.original).map((meta, idx) => (
                  <React.Fragment key={idx}>
                    {meta.type}
                    <br />
                  </React.Fragment>
                ))}
              </Table.Td>
              <Table.Td c="pink.9">
                {getMeta(m.original).map((meta, idx) => (
                  <React.Fragment key={idx}>
                    {meta.help}
                    <br />
                  </React.Fragment>
                ))}
              </Table.Td>
            </Table.Tr>
          ))}
        </Table.Tbody>
      </Table>
    </Stack>
  );
};

export default MetricsExplorer;
