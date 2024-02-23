import {
  Card,
  Group,
  Table,
  Text,
  Accordion,
  Badge,
  Tooltip,
  Box,
  Switch,
} from "@mantine/core";
import { useSuspenseAPIQuery } from "../api/api";
import { AlertingRulesMap } from "../api/response-types/rules";
import badgeClasses from "../badge.module.css";
import RuleDefinition from "../rule-definition";
import { formatRelative, now } from "../lib/time-format";
import { Fragment, useState } from "react";

export default function Alerts() {
  const { data } = useSuspenseAPIQuery<AlertingRulesMap>(`/rules?type=alert`);
  const [showAnnotations, setShowAnnotations] = useState(false);

  const ruleStatsCount = {
    inactive: 0,
    pending: 0,
    firing: 0,
  };

  data.data.groups.forEach((el) =>
    el.rules.forEach((r) => ruleStatsCount[r.state]++)
  );

  return (
    <>
      <Switch
        checked={showAnnotations}
        label="Show annotations"
        onChange={(event) => setShowAnnotations(event.currentTarget.checked)}
        mb="md"
      />
      {data.data.groups.map((g, i) => (
        <Card
          shadow="xs"
          withBorder
          radius="md"
          p="md"
          mb="md"
          key={i} // TODO: Find a stable and definitely unique key.
        >
          <Group mb="md" mt="xs" ml="xs" justify="space-between">
            <Group align="baseline">
              <Text fz="xl" fw={600} c="var(--mantine-primary-color-filled)">
                {g.name}
              </Text>
              <Text fz="sm" c="gray.6">
                {g.file}
              </Text>
            </Group>
          </Group>
          <Accordion multiple variant="separated">
            {g.rules.map((r, j) => {
              const numFiring = r.alerts.filter(
                (a) => a.state === "firing"
              ).length;
              const numPending = r.alerts.filter(
                (a) => a.state === "pending"
              ).length;

              return (
                <Accordion.Item key={j} value={j.toString()}>
                  <Accordion.Control>
                    <Group wrap="nowrap" justify="space-between" mr="lg">
                      <Text>{r.name}</Text>
                      <Group gap="xs">
                        {numFiring > 0 && (
                          <Badge className={badgeClasses.healthErr}>
                            firing ({numFiring})
                          </Badge>
                        )}
                        {numPending > 0 && (
                          <Badge className={badgeClasses.healthWarn}>
                            pending ({numPending})
                          </Badge>
                        )}
                        {/* {numFiring === 0 && numPending === 0 && (
                          <Badge className={badgeClasses.healthOk}>
                            inactive
                          </Badge>
                        )} */}
                      </Group>
                    </Group>
                  </Accordion.Control>
                  <Accordion.Panel>
                    <RuleDefinition rule={r} />
                    {r.alerts.length > 0 && (
                      <Table mt="lg">
                        <Table.Thead>
                          <Table.Tr>
                            <Table.Th>Alert labels</Table.Th>
                            <Table.Th>State</Table.Th>
                            <Table.Th>Active Since</Table.Th>
                            <Table.Th>Value</Table.Th>
                          </Table.Tr>
                        </Table.Thead>
                        <Table.Tbody>
                          {r.type === "alerting" &&
                            r.alerts.map((a, k) => (
                              <Fragment key={k}>
                                <Table.Tr>
                                  <Table.Td>
                                    <Group gap="xs">
                                      {Object.entries(a.labels).map(
                                        ([k, v]) => {
                                          return (
                                            <Badge
                                              variant="light"
                                              className={
                                                badgeClasses.labelBadge
                                              }
                                              styles={{
                                                label: {
                                                  textTransform: "none",
                                                },
                                              }}
                                              key={k}
                                            >
                                              {/* TODO: Proper quote escaping */}
                                              {k}="{v}"
                                            </Badge>
                                          );
                                        }
                                      )}
                                    </Group>
                                  </Table.Td>
                                  <Table.Td>
                                    <Badge
                                      className={
                                        a.state === "firing"
                                          ? badgeClasses.healthErr
                                          : badgeClasses.healthWarn
                                      }
                                    >
                                      {a.state}
                                    </Badge>
                                  </Table.Td>
                                  <Table.Td>
                                    <Tooltip label={a.activeAt}>
                                      <Box>
                                        {formatRelative(a.activeAt, now(), "")}
                                      </Box>
                                    </Tooltip>
                                  </Table.Td>
                                  <Table.Td>{a.value}</Table.Td>
                                </Table.Tr>
                                {showAnnotations && (
                                  <Table.Tr>
                                    <Table.Td colSpan={4}>
                                      <Table mt="md" mb="xl">
                                        <Table.Tbody>
                                          {Object.entries(a.annotations).map(
                                            ([k, v]) => (
                                              <Table.Tr key={k}>
                                                <Table.Th>{k}</Table.Th>
                                                <Table.Td>{v}</Table.Td>
                                              </Table.Tr>
                                            )
                                          )}
                                        </Table.Tbody>
                                      </Table>
                                    </Table.Td>
                                  </Table.Tr>
                                )}
                              </Fragment>
                            ))}
                        </Table.Tbody>
                      </Table>
                    )}
                  </Accordion.Panel>
                </Accordion.Item>
              );
            })}
          </Accordion>
        </Card>
      ))}
    </>
  );
}
