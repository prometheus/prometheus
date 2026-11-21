import {
  ActionIcon,
  Indicator,
  Popover,
  Card,
  Text,
  Stack,
  ScrollArea,
  Group,
  rem,
} from "@mantine/core";
import {
  IconAlertTriangle,
  IconNetworkOff,
  IconMessageExclamation,
} from "@tabler/icons-react";
import { useNotifications } from "../state/useNotifications";
import { actionIconStyle } from "../styles";
import { useSettings } from "../state/settingsSlice";
import { formatTimestamp } from "../lib/formatTime";
import { useI18n } from "../i18nContext";

const NotificationsIcon = () => {
  const { notifications, isConnectionError } = useNotifications();
  const { useLocalTime } = useSettings();
  const { t } = useI18n();

  return notifications.length === 0 && !isConnectionError ? null : (
    <Indicator
      color={"red"}
      size={16}
      label={isConnectionError ? "!" : notifications.length}
    >
      <Popover position="bottom-end" shadow="md" withArrow>
        <Popover.Target>
          <ActionIcon
            color="gray"
            title={t("notifications.notifications")}
            aria-label={t("notifications.notifications")}
            size={32}
          >
            <IconMessageExclamation style={actionIconStyle} />
          </ActionIcon>
        </Popover.Target>

        <Popover.Dropdown>
          <Stack gap="xs">
            <Text fw={700} size="xs" c="dimmed" ta="center">
              {t("notifications.notifications")}
            </Text>
            <ScrollArea.Autosize mah={200}>
              {isConnectionError ? (
                <Card p="xs" color="red">
                  <Group wrap="nowrap">
                    <IconNetworkOff
                      color="red"
                      style={{ width: rem(20), height: rem(20) }}
                    />
                    <Stack gap="0">
                      <Text size="sm" fw={500}>
                        {t("notifications.connectionInterrupted")}
                      </Text>
                      <Text size="xs" c="dimmed">
                        {t("notifications.refreshOrCheckConnection")}
                      </Text>
                    </Stack>
                  </Group>
                </Card>
              ) : notifications.length === 0 ? (
                <Text ta="center" c="dimmed">
                  {t("notifications.noNotifications")}
                </Text>
              ) : (
                notifications.map((notification, index) => (
                  <Card key={index} p="xs">
                    <Group wrap="nowrap" align="flex-start">
                      <IconAlertTriangle
                        color="red"
                        style={{
                          width: rem(20),
                          height: rem(20),
                          marginTop: rem(3),
                        }}
                      />
                      <Stack maw={250} gap={5}>
                        <Text size="sm" fw={500}>
                          {notification.text}
                        </Text>
                        <Text size="xs" c="dimmed">
                          {formatTimestamp(
                            new Date(notification.date).valueOf() / 1000,
                            useLocalTime
                          )}
                        </Text>
                      </Stack>
                    </Group>
                  </Card>
                ))
              )}
            </ScrollArea.Autosize>
          </Stack>
        </Popover.Dropdown>
      </Popover>
    </Indicator>
  );
};

export default NotificationsIcon;
