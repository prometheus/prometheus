import { FC } from "react";
import { Labels } from "../../api/responseTypes/targets";
import { LabelBadges } from "../../components/LabelBadges";
import { ActionIcon, Collapse, Group, Stack, Text } from "@mantine/core";
import { useDisclosure } from "@mantine/hooks";
import { IconChevronDown, IconChevronUp } from "@tabler/icons-react";
import { actionIconStyle } from "../../styles";

type TargetLabelsProps = {
  labels: Labels;
  discoveredLabels: Labels;
};

const TargetLabels: FC<TargetLabelsProps> = ({ discoveredLabels, labels }) => {
  const [showDiscovered, { toggle: toggleDiscovered }] = useDisclosure(false);

  return (
    <Stack>
      <Group wrap="nowrap" align="flex-start">
        <LabelBadges labels={labels} />

        <ActionIcon
          size="xs"
          color="gray"
          variant="light"
          onClick={toggleDiscovered}
          title={`${showDiscovered ? "Hide" : "Show"} discovered (pre-relabeling) labels`}
        >
          {showDiscovered ? (
            <IconChevronUp style={actionIconStyle} />
          ) : (
            <IconChevronDown style={actionIconStyle} />
          )}
        </ActionIcon>
      </Group>

      <Collapse in={showDiscovered}>
        {/* Additionally remove DOM elements when not expanded (helps performance) */}
        {showDiscovered && (
          <>
            <Text fw={700} size="1em" my="lg" c="gray.7">
              Discovered labels:
            </Text>
            <LabelBadges
              labels={discoveredLabels}
              style={{
                color: "var(--mantine-color-blue-light-color)",
                backgroundColor: "var(--mantine-color-blue-light)",
              }}
            />
          </>
        )}
      </Collapse>
    </Stack>
  );
};

export default TargetLabels;
