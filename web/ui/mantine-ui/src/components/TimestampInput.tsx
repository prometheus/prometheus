import { ActionIcon, Group } from "@mantine/core";
import {
  DatePicker,
  PickerInputBase,
  PickerInputBaseProps,
  TimePicker,
} from "@mantine/dates";
import { useDidUpdate, useDisclosure } from "@mantine/hooks";
import { IconCheck } from "@tabler/icons-react";
import dayjs from "dayjs";
import utc from "dayjs/plugin/utc";
import { useRef, useState } from "react";
import { useSettings } from "../state/settingsSlice";

dayjs.extend(utc);

interface TimestampInputProps extends Pick<
  PickerInputBaseProps,
  | "aria-label"
  | "clearable"
  | "description"
  | "disabled"
  | "label"
  | "leftSection"
  | "onClick"
  | "placeholder"
  | "rightSection"
  | "styles"
  | "title"
  | "w"
> {
  value: number | null;
  onChange: (value: number | null) => void;
}

/** TimestampInput edits a timestamp in UTC or the configured local timezone. */
export default function TimestampInput({
  value,
  onChange,
  disabled,
  ...props
}: TimestampInputProps) {
  const { useLocalTime } = useSettings();
  const [opened, handlers] = useDisclosure(false);
  const hoursRef = useRef<HTMLInputElement>(null);
  const formatted =
    value === null
      ? null
      : (useLocalTime ? dayjs(value) : dayjs.utc(value)).format(
          "YYYY-MM-DD HH:mm:ss",
        );
  const date = formatted?.slice(0, 10) ?? null;
  const time = formatted?.slice(11) ?? "";
  const [timeInput, setTimeInput] = useState(time);
  useDidUpdate(() => setTimeInput(time), [date, time, opened]);
  useDidUpdate(() => {
    // Echo complete edits before syncing DST normalization, even if the timestamp is unchanged.
    if (timeInput) {
      setTimeInput(time);
    }
  }, [time, timeInput]);
  const today = (useLocalTime ? dayjs() : dayjs.utc()).format("YYYY-MM-DD");
  const change = (date: string, time: string) => {
    const text = `${date} ${time}`;
    onChange((useLocalTime ? dayjs(text) : dayjs.utc(text)).valueOf());
  };

  // Keep the time out of Mantine's date parsing, which normalizes local DST gaps.
  return (
    <PickerInputBase
      {...props}
      disabled={disabled}
      type="default"
      dropdownType="popover"
      value={date}
      formattedValue={formatted}
      dropdownOpened={!disabled && opened}
      dropdownHandlers={handlers}
      onClear={() => onChange(null)}
      shouldClear={value !== null}
      clearButtonProps={{ "aria-label": "Clear time" }}
    >
      <DatePicker
        value={date}
        defaultDate={date ?? today}
        onChange={(date) => {
          if (date) {
            change(date, time || "00:00:00");
            hoursRef.current?.focus();
          }
        }}
      />
      <Group mt="sm" gap="sm" wrap="nowrap">
        {/* Reset partial fields even if reopening interrupts the closing transition. */}
        <TimePicker
          key={opened ? "open" : "closed"}
          flex={1}
          value={timeInput}
          onChange={(time) => {
            setTimeInput(time);
            if (time) {
              change(date ?? today, time);
            }
          }}
          withSeconds
          hoursRef={hoursRef}
          hoursInputLabel="Hours"
          minutesInputLabel="Minutes"
          secondsInputLabel="Seconds"
          onKeyDown={(event) => {
            if (event.key === "Enter") {
              event.preventDefault();
              handlers.close();
            }
          }}
        />
        <ActionIcon
          variant="default"
          size="input-sm"
          aria-label="Apply time"
          onClick={handlers.close}
        >
          <IconCheck size={16} />
        </ActionIcon>
      </Group>
    </PickerInputBase>
  );
}
