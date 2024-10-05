import { FC, useState } from "react";
import { Select, TextInput } from "@mantine/core";
import {
  formatPrometheusDuration,
  parsePrometheusDuration,
} from "../../lib/formatTime";
import {
  GraphResolution,
  getEffectiveResolution,
} from "../../state/queryPageSlice";

interface ResolutionInputProps {
  resolution: GraphResolution;
  range: number;
  onChangeResolution: (resolution: GraphResolution) => void;
}

const ResolutionInput: FC<ResolutionInputProps> = ({
  resolution,
  range,
  onChangeResolution,
}) => {
  const [customResolutionInput, setCustomResolutionInput] = useState<string>(
    formatPrometheusDuration(getEffectiveResolution(resolution, range))
  );

  const onChangeCustomResolutionInput = (resText: string): void => {
    const newResolution = parsePrometheusDuration(resText);

    if (resolution.type === "custom" && newResolution === resolution.step) {
      // Nothing changed.
      return;
    }

    if (newResolution === null) {
      setCustomResolutionInput(
        formatPrometheusDuration(getEffectiveResolution(resolution, range))
      );
    } else {
      onChangeResolution({ type: "custom", step: newResolution });
    }
  };

  return (
    <>
      <Select
        title="Resolution"
        placeholder="Resolution"
        maxDropdownHeight={500}
        data={[
          {
            group: "Automatic resolution",
            items: [
              { label: "Low res.", value: "low" },
              { label: "Medium res.", value: "medium" },
              { label: "High res.", value: "high" },
            ],
          },
          {
            group: "Fixed resolution",
            items: [
              { label: "10s", value: "10000" },
              { label: "30s", value: "30000" },
              { label: "1m", value: "60000" },
              { label: "5m", value: "300000" },
              { label: "15m", value: "900000" },
              { label: "1h", value: "3600000" },
            ],
          },
          {
            group: "Custom resolution",
            items: [{ label: "Enter value...", value: "custom" }],
          },
        ]}
        w={160}
        value={
          resolution.type === "auto"
            ? resolution.density
            : resolution.type === "fixed"
              ? resolution.step.toString()
              : "custom"
        }
        onChange={(_value, option) => {
          if (["low", "medium", "high"].includes(option.value)) {
            onChangeResolution({
              type: "auto",
              density: option.value as "low" | "medium" | "high",
            });
            return;
          }

          if (option.value === "custom") {
            // Start the custom resolution at the current effective resolution.
            const effectiveResolution = getEffectiveResolution(
              resolution,
              range
            );
            onChangeResolution({
              type: "custom",
              step: effectiveResolution,
            });
            setCustomResolutionInput(
              formatPrometheusDuration(effectiveResolution)
            );
            return;
          }

          const value = parseInt(option.value);
          if (!isNaN(value)) {
            onChangeResolution({
              type: "fixed",
              step: value,
            });
          } else {
            throw new Error("Invalid resolution value");
          }
        }}
      />

      {resolution.type === "custom" && (
        <TextInput
          placeholder="Resolution"
          value={customResolutionInput}
          onChange={(event) =>
            setCustomResolutionInput(event.currentTarget.value)
          }
          onBlur={() => onChangeCustomResolutionInput(customResolutionInput)}
          onKeyDown={(event) =>
            event.key === "Enter" &&
            onChangeCustomResolutionInput(customResolutionInput)
          }
          aria-label="Range"
          style={{
            width: `calc(44px + ${customResolutionInput.length + 3}ch)`,
          }}
        />
      )}
    </>
  );
};

export default ResolutionInput;
