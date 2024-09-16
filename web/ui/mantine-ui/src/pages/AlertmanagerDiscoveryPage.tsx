import { Alert, Table } from "@mantine/core";
import { IconBell, IconBellOff, IconInfoCircle } from "@tabler/icons-react";

import { useSuspenseAPIQuery } from "../api/api";
import { AlertmanagersResult } from "../api/responseTypes/alertmanagers";
import EndpointLink from "../components/EndpointLink";
import InfoPageCard from "../components/InfoPageCard";
import InfoPageStack from "../components/InfoPageStack";

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
    <InfoPageStack>
      <InfoPageCard title="Active Alertmanagers" icon={IconBell}>
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
      </InfoPageCard>
      <InfoPageCard title="Dropped Alertmanagers" icon={IconBellOff}>
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
      </InfoPageCard>
    </InfoPageStack>
  );
}
