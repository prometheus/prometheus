import { Alert, Table } from "@mantine/core";
import { IconBell, IconBellOff, IconInfoCircle } from "@tabler/icons-react";

import { useSuspenseAPIQuery } from "../api/api";
import { AlertmanagersResult } from "../api/responseTypes/alertmanagers";
import EndpointLink from "../components/EndpointLink";
import InfoPageCard from "../components/InfoPageCard";
import InfoPageStack from "../components/InfoPageStack";
import { useI18n } from "../i18nContext";

export const targetPoolDisplayLimit = 20;

export default function AlertmanagerDiscoveryPage() {
  const { t } = useI18n();
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
      <InfoPageCard title={t("alertmanagers.active")} icon={IconBell}>
        {activeAlertmanagers.length === 0 ? (
          <Alert title={t("alertmanagers.noActive")} icon={<IconInfoCircle />}>
            {t("alertmanagers.noActiveFound")}
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
      <InfoPageCard title={t("alertmanagers.dropped")} icon={IconBellOff}>
        {droppedAlertmanagers.length === 0 ? (
          <Alert title={t("alertmanagers.noDropped")} icon={<IconInfoCircle />}>
            {t("alertmanagers.noDroppedFound")}
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
