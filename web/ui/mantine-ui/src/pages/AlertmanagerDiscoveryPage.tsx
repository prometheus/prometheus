import {
  ActionIcon,
  Alert,
  Box,
  Card,
  Group,
  Select,
  Skeleton,
  Stack,
  Table,
  Text,
} from "@mantine/core";
import {
  IconBell,
  IconBellOff,
  IconInfoCircle,
  IconLayoutNavbarCollapse,
  IconLayoutNavbarExpand,
  IconSearch,
} from "@tabler/icons-react";
import { Suspense } from "react";
import { useAppDispatch, useAppSelector } from "../state/hooks";

import {
  ArrayParam,
  StringParam,
  useQueryParam,
  withDefault,
} from "use-query-params";
import ErrorBoundary from "../components/ErrorBoundary";
import ScrapePoolList from "./ServiceDiscoveryPoolsList";
import { useSuspenseAPIQuery } from "../api/api";
import { ScrapePoolsResult } from "../api/responseTypes/scrapePools";
import {
  setCollapsedPools,
  setShowLimitAlert,
} from "../state/serviceDiscoveryPageSlice";
import { StateMultiSelect } from "../components/StateMultiSelect";
import badgeClasses from "../Badge.module.css";
import { AlertmanagersResult } from "../api/responseTypes/alertmanagers";
import EndpointLink from "../components/EndpointLink";

export const targetPoolDisplayLimit = 20;

export default function AlertmanagerDiscoveryPage() {
  // Load the list of all available scrape pools.
  const {
    data: {
      data: { activeAlertmanagers, droppedAlertmanagers },
    },
  } = useSuspenseAPIQuery<AlertmanagersResult>({
    path: `/alertmanagers`,
  });

  return (
    <Stack gap="lg" maw={1000} mx="auto" mt="xs">
      <Card shadow="xs" withBorder p="md">
        <Group wrap="nowrap" align="center" ml="xs" mb="sm" gap="xs">
          <IconBell size={22} />
          <Text fz="xl" fw={600}>
            Active Alertmanagers
          </Text>
        </Group>
        {activeAlertmanagers.length === 0 ? (
          <Alert title="No active alertmanagers" icon={<IconInfoCircle />}>
            No active alertmanagers found.
          </Alert>
        ) : (
          <Table layout="fixed">
            <Table.Tbody>
              {activeAlertmanagers.map((alertmanager) => (
                <Table.Tr key={alertmanager.url}>
                  <Table.Td>
                    <EndpointLink
                      endpoint={alertmanager.url}
                      globalUrl={alertmanager.url}
                    />
                  </Table.Td>
                </Table.Tr>
              ))}
            </Table.Tbody>
          </Table>
        )}
      </Card>
      <Card shadow="xs" withBorder p="md">
        <Group wrap="nowrap" align="center" ml="xs" mb="sm" gap="xs">
          <IconBellOff size={22} />
          <Text fz="xl" fw={600}>
            Dropped Alertmanagers
          </Text>
        </Group>
        {droppedAlertmanagers.length === 0 ? (
          <Alert title="No dropped alertmanagers" icon={<IconInfoCircle />}>
            No dropped alertmanagers found.
          </Alert>
        ) : (
          <Table layout="fixed">
            <Table.Tbody>
              {droppedAlertmanagers.map((alertmanager) => (
                <Table.Tr key={alertmanager.url}>
                  <Table.Td>
                    <EndpointLink
                      endpoint={alertmanager.url}
                      globalUrl={alertmanager.url}
                    />
                  </Table.Td>
                </Table.Tr>
              ))}
            </Table.Tbody>
          </Table>
        )}
      </Card>
    </Stack>
  );
}
