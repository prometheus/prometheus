import { Text } from "@mantine/core";
import { IconSpy } from "@tabler/icons-react";
import { FC } from "react";
import InfoPageStack from "../components/InfoPageStack";
import InfoPageCard from "../components/InfoPageCard";
import { useI18n } from "../i18nContext";

const AgentPage: FC = () => {
  const { t } = useI18n();
  return (
    <InfoPageStack>
      <InfoPageCard
        title={t("agent.title")}
        icon={IconSpy}
      >
      <Text p="md">
        {t("agent.description")}
      </Text>
      <Text p="md">
        {t("agent.limitations")}
        </Text>
      </InfoPageCard>
    </InfoPageStack>
  );
};

export default AgentPage;
