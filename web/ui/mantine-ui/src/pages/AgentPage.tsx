import { Card, Group, Text } from "@mantine/core";
import { IconSpy } from "@tabler/icons-react";
import { FC } from "react";

const AgentPage: FC = () => {
  return (
    <Card shadow="xs" withBorder p="md" mt="xs">
      <Group wrap="nowrap" align="center" ml="xs" mb="sm" gap="xs">
        <IconSpy size={22} />
        <Text fz="xl" fw={600}>
          Prometheus Agent
        </Text>
      </Group>
      <Text p="md">
        This Prometheus instance is running in <strong>agent mode</strong>. In
        this mode, Prometheus is only used to scrape discovered targets and
        forward the scraped metrics to remote write endpoints.
      </Text>
      <Text p="md">
        Some features are not available in this mode, such as querying and
        alerting.
      </Text>
    </Card>
  );
};

export default AgentPage;
