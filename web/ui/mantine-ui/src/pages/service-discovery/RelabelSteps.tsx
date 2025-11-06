import { em, Group, Stack, Table, Text } from "@mantine/core";
import { useSuspenseAPIQuery } from "../../api/api";
import { RelabelStepsResult } from "../../api/responseTypes/relabel_steps";
import { Labels } from "../../api/responseTypes/targets";
import React from "react";
import {
  IconArrowDown,
  IconCircleMinus,
  IconCirclePlus,
  IconReplace,
  IconTags,
} from "@tabler/icons-react";

const iconStyle = { width: em(18), height: em(18), verticalAlign: "middle" };

const ruleTable = (rule: { [key: string]: unknown }, idx: number) => {
  return (
    <Table
      w="60%"
      withTableBorder
      withColumnBorders
      bg="light-dark(var(--mantine-color-gray-0), var(--mantine-color-gray-8))"
    >
      <Table.Thead>
        <Table.Tr
          bg={
            "light-dark(var(--mantine-color-gray-1), var(--mantine-color-gray-7))"
          }
        >
          <Table.Th colSpan={2}>
            <Group gap="xs">
              <IconReplace style={iconStyle} /> Rule {idx + 1}
            </Group>
          </Table.Th>
        </Table.Tr>
      </Table.Thead>
      <Table.Tbody>
        {Object.entries(rule)
          .sort(([a], [b]) => {
            // Sort by a predefined order for known fields, otherwise alphabetically.
            const sortedRuleFieldNames: string[] = [
              "action",
              "source_labels",
              "regex",
              "modulus",
              "replacement",
              "target_label",
            ];
            const ai = sortedRuleFieldNames.indexOf(a);
            const bi = sortedRuleFieldNames.indexOf(b);
            if (ai === -1 && bi === -1) {
              return a.localeCompare(b);
            }
            if (ai === -1) {
              return 1;
            }
            if (bi === -1) {
              return -1;
            }
            return ai - bi;
          })
          .map(([k, v]) => (
            <Table.Tr key={k}>
              <Table.Th>
                <code>{k}</code>
              </Table.Th>
              <Table.Td>
                <code>
                  {typeof v === "object" ? JSON.stringify(v) : String(v)}
                </code>
              </Table.Td>
            </Table.Tr>
          ))}
      </Table.Tbody>
    </Table>
  );
};

const labelsTable = (labels: Labels, prevLabels: Labels | null) => {
  if (labels === null) {
    return <Text c="dimmed">dropped</Text>;
  }

  return (
    <Table w="50%" withTableBorder>
      <Table.Thead>
        <Table.Tr
          bg={
            "light-dark(var(--mantine-color-gray-1), var(--mantine-color-gray-7))"
          }
        >
          <Table.Th colSpan={3}>
            <Group gap="xs">
              <IconTags style={iconStyle} /> Labels
            </Group>
          </Table.Th>
        </Table.Tr>
      </Table.Thead>
      <Table.Tbody>
        {Object.entries(labels)
          .concat(
            prevLabels
              ? Object.entries(prevLabels).filter(
                  ([k]) => labels[k] === undefined
                )
              : []
          )
          .sort(([a], [b]) => a.localeCompare(b))
          .map(([k, v]) => {
            const added = prevLabels !== null && prevLabels[k] === undefined;
            const changed =
              prevLabels !== null && !added && prevLabels[k] !== v;
            const removed =
              prevLabels !== null &&
              !changed &&
              prevLabels[k] !== undefined &&
              labels[k] === undefined;
            return (
              <Table.Tr
                key={k}
                bg={
                  added
                    ? "light-dark(var(--mantine-color-green-1), var(--mantine-color-green-8))"
                    : changed
                      ? "light-dark(var(--mantine-color-blue-1), var(--mantine-color-blue-8))"
                      : removed
                        ? "light-dark(var(--mantine-color-red-1), var(--mantine-color-red-8))"
                        : undefined
                }
              >
                <Table.Td w={40}>
                  {added ? (
                    <IconCirclePlus style={iconStyle} />
                  ) : changed ? (
                    <IconReplace style={iconStyle} />
                  ) : removed ? (
                    <IconCircleMinus style={iconStyle} />
                  ) : null}
                </Table.Td>
                <Table.Th>
                  <code>{k}</code>
                </Table.Th>
                <Table.Td>
                  <code>{v}</code>
                </Table.Td>
              </Table.Tr>
            );
          })}
      </Table.Tbody>
    </Table>
  );
};

const RelabelSteps: React.FC<{
  labels: Labels;
  pool: string;
}> = ({ labels, pool }) => {
  const { data } = useSuspenseAPIQuery<RelabelStepsResult>({
    path: `/targets/relabel_steps`,
    params: {
      labels: JSON.stringify(labels),
      scrapePool: pool,
    },
  });

  return (
    <Stack align="center">
      {labelsTable(labels, null)}
      {data.data.steps.map((step, idx) => (
        <React.Fragment key={idx}>
          <IconArrowDown style={iconStyle} />
          {ruleTable(step.rule, idx)}
          <IconArrowDown style={iconStyle} />
          {step.keep ? (
            labelsTable(
              step.output,
              idx === 0 ? labels : data.data.steps[idx - 1].output
            )
          ) : (
            <Text c="dimmed">dropped</Text>
          )}
        </React.Fragment>
      ))}
    </Stack>
  );
};

export default RelabelSteps;
