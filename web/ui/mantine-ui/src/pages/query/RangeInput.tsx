import { FC, useState } from "react";
import { ActionIcon, Group, Input } from "@mantine/core";
import { IconMinus, IconPlus } from "@tabler/icons-react";
import { formatDuration, parseDuration } from "../../lib/formatTime";

interface RangeInputProps {
  range: number;
  onChangeRange: (range: number) => void;
}

const iconStyle = { width: "0.9rem", height: "0.9rem" };

const rangeSteps = [
  1,
  10,
  60,
  5 * 60,
  15 * 60,
  30 * 60,
  60 * 60,
  2 * 60 * 60,
  6 * 60 * 60,
  12 * 60 * 60,
  24 * 60 * 60,
  48 * 60 * 60,
  7 * 24 * 60 * 60,
  14 * 24 * 60 * 60,
  28 * 24 * 60 * 60,
  56 * 24 * 60 * 60,
  112 * 24 * 60 * 60,
  182 * 24 * 60 * 60,
  365 * 24 * 60 * 60,
  730 * 24 * 60 * 60,
].map((s) => s * 1000);

const RangeInput: FC<RangeInputProps> = ({ range, onChangeRange }) => {
  // TODO: Make sure that when "range" changes externally (like via the URL),
  // the input is updated, either via useEffect() or some better architecture.
  const [rangeInput, setRangeInput] = useState<string>(formatDuration(range));

  const onChangeRangeInput = (rangeText: string): void => {
    const newRange = parseDuration(rangeText);
    if (newRange === null) {
      setRangeInput(formatDuration(range));
    } else {
      onChangeRange(newRange);
    }
  };

  const increaseRange = (): void => {
    for (const step of rangeSteps) {
      if (range < step) {
        setRangeInput(formatDuration(step));
        onChangeRange(step);
        return;
      }
    }
  };

  const decreaseRange = (): void => {
    for (const step of rangeSteps.slice().reverse()) {
      if (range > step) {
        setRangeInput(formatDuration(step));
        onChangeRange(step);
        return;
      }
    }
  };

  return (
    <Group gap={5}>
      <ActionIcon
        size="lg"
        variant="subtle"
        aria-label="Decrease range"
        onClick={decreaseRange}
      >
        <IconMinus style={iconStyle} />
      </ActionIcon>
      <Input
        value={rangeInput}
        onChange={(event) => setRangeInput(event.currentTarget.value)}
        onBlur={() => onChangeRangeInput(rangeInput)}
        onKeyDown={(event) =>
          event.key === "Enter" && onChangeRangeInput(rangeInput)
        }
        aria-label="Range"
        style={{ width: rangeInput.length + 3 + "ch" }}
      />
      <ActionIcon
        size="lg"
        variant="subtle"
        aria-label="Increase range"
        onClick={increaseRange}
      >
        <IconPlus style={iconStyle} />
      </ActionIcon>
    </Group>
  );
};

export default RangeInput;
