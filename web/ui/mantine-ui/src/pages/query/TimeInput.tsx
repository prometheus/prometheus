import { Group, ActionIcon } from "@mantine/core";
import { DatesProvider, DateTimePicker } from "@mantine/dates";
import { IconChevronLeft, IconChevronRight } from "@tabler/icons-react";
import { FC } from "react";

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

  return (
    <Group gap={5}>
      <ActionIcon
        size="lg"
        variant="subtle"
        title="Decrease time"
        aria-label="Decrease time"
        onClick={() => onChangeTime(baseTime() - range / 2)}
      >
        <IconChevronLeft style={iconStyle} />
      </ActionIcon>
      <DatesProvider settings={{ timezone: "UTC" }}>
        <DateTimePicker
          w={180}
          valueFormat="YYYY-MM-DD HH:mm:ss"
          withSeconds
          clearable
          value={time !== null ? new Date(time) : undefined}
          onChange={(value) => onChangeTime(value ? value.getTime() : null)}
          aria-label={description}
          placeholder={description}
          onClick={() => {
            if (time === null) {
              onChangeTime(baseTime());
            }
          }}
        />
      </DatesProvider>
      <ActionIcon
        size="lg"
        variant="subtle"
        title="Increase time"
        aria-label="Increase time"
        onClick={() => onChangeTime(baseTime() + range / 2)}
      >
        <IconChevronRight style={iconStyle} />
      </ActionIcon>
    </Group>
  );
};

export default TimeInput;
