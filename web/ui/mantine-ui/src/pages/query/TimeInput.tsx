import { Group, ActionIcon, CloseButton } from "@mantine/core";
import { DatesProvider, DateTimePicker } from "@mantine/dates";
import { IconChevronLeft, IconChevronRight } from "@tabler/icons-react";
import { FC } from "react";
import { useSettings } from "../../state/settingsSlice";

interface TimeInputProps {
  time: number | null; // Timestamp in milliseconds.
  range: number; // Range in seconds.
  description: string;
  onChangeTime: (time: number | null) => void;
}

const iconStyle = { width: "0.9rem", height: "0.9rem" };

const TimeInput: FC<TimeInputProps> = ({
  time,
  range,
  description,
  onChangeTime,
}) => {
  const baseTime = () => (time !== null ? time : Date.now().valueOf());
  const { useLocalTime } = useSettings();

  return (
    <Group gap={5}>
      <DatesProvider settings={{ timezone: useLocalTime ? undefined : "UTC" }}>
        <DateTimePicker
          title="End time"
          w={230}
          valueFormat="YYYY-MM-DD HH:mm:ss"
          withSeconds
          // clearable
          value={time !== null ? new Date(time) : undefined}
          onChange={(value) => onChangeTime(value ? value.getTime() : null)}
          aria-label={description}
          placeholder={description}
          onClick={() => {
            if (time === null) {
              onChangeTime(baseTime());
            }
          }}
          leftSection={
            <ActionIcon
              size="lg"
              color="gray"
              variant="transparent"
              title="Decrease time"
              aria-label="Decrease time"
              onClick={() => onChangeTime(baseTime() - range / 2)}
            >
              <IconChevronLeft style={iconStyle} />
            </ActionIcon>
          }
          styles={{ section: { width: "unset" } }}
          rightSection={
            <>
              {time && (
                <CloseButton
                  variant="transparent"
                  color="gray"
                  onMouseDown={(event) => event.preventDefault()}
                  tabIndex={-1}
                  onClick={() => {
                    onChangeTime(null);
                  }}
                  size="xs"
                />
              )}
              <ActionIcon
                size="lg"
                color="gray"
                variant="transparent"
                title="Increase time"
                aria-label="Increase time"
                onClick={() => onChangeTime(baseTime() + range / 2)}
              >
                <IconChevronRight style={iconStyle} />
              </ActionIcon>
            </>
          }
        />
      </DatesProvider>
    </Group>
  );
};

export default TimeInput;
