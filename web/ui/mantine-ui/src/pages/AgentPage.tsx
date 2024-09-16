import { Text } from "@mantine/core";
import { IconSpy } from "@tabler/icons-react";
import { FC } from "react";
import InfoPageStack from "../components/InfoPageStack";
import InfoPageCard from "../components/InfoPageCard";

const AgentPage: FC = () => {
  return (
    <InfoPageStack>
      <InfoPageCard
        title="Prometheus Agent"
        icon={IconSpy}
      >
      <Text p="md">
        This Prometheus instance is running in <strong>agent mode</strong>. In
        this mode, Prometheus is only used to scrape discovered targets and
        forward the scraped metrics to remote write endpoints.
      </Text>
      <Text p="md">
        Some features are not available in this mode, such as querying and
          alerting.
        </Text>
      </InfoPageCard>
    </InfoPageStack>
  );
};

export default AgentPage;
