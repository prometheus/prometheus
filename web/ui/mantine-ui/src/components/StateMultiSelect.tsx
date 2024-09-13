import { FC } from "react";
import {
  CheckIcon,
  Combobox,
  ComboboxChevron,
  ComboboxClearButton,
  Group,
  Pill,
  PillsInput,
  useCombobox,
} from "@mantine/core";
import { IconHeartRateMonitor } from "@tabler/icons-react";
import { inputIconStyle } from "../styles";

interface StatePillProps extends React.ComponentPropsWithoutRef<"div"> {
  value: string;
  onRemove?: () => void;
}

export function StatePill({ value, onRemove, ...others }: StatePillProps) {
  return (
    <Pill
      fw={600}
      style={{ textTransform: "uppercase", fontWeight: 600 }}
      onRemove={onRemove}
      {...others}
      withRemoveButton={!!onRemove}
    >
      {value}
    </Pill>
  );
}

interface StateMultiSelectProps {
  options: string[];
  optionClass: (option: string) => string;
  optionCount?: (option: string) => number;
  placeholder: string;
  values: string[];
  onChange: (values: string[]) => void;
}

export const StateMultiSelect: FC<StateMultiSelectProps> = ({
  options,
  optionClass,
  optionCount,
  placeholder,
  values,
  onChange,
}) => {
  const combobox = useCombobox({
    onDropdownClose: () => combobox.resetSelectedOption(),
    onDropdownOpen: () => combobox.updateSelectedOptionIndex("active"),
  });

  const handleValueSelect = (val: string) =>
    onChange(
      values.includes(val) ? values.filter((v) => v !== val) : [...values, val]
    );

  const handleValueRemove = (val: string) =>
    onChange(values.filter((v) => v !== val));

  const renderedValues = values.map((item) => (
    <StatePill
      value={optionCount ? `${item} (${optionCount(item)})` : item}
      className={optionClass(item)}
      onRemove={() => handleValueRemove(item)}
      key={item}
    />
  ));

  return (
    <Combobox
      store={combobox}
      onOptionSubmit={handleValueSelect}
      withinPortal={false}
    >
      <Combobox.DropdownTarget>
        <PillsInput
          pointer
          onClick={() => combobox.toggleDropdown()}
          miw={200}
          leftSection={<IconHeartRateMonitor style={inputIconStyle} />}
          rightSection={
            values.length > 0 ? (
              <ComboboxClearButton onClear={() => onChange([])} />
            ) : (
              <ComboboxChevron />
            )
          }
        >
          <Pill.Group>
            {renderedValues.length > 0 ? (
              renderedValues
            ) : (
              <PillsInput.Field placeholder={placeholder} mt={1} />
            )}

            <Combobox.EventsTarget>
              <PillsInput.Field
                type="hidden"
                onBlur={() => combobox.closeDropdown()}
                onKeyDown={(event) => {
                  if (event.key === "Backspace") {
                    event.preventDefault();
                    handleValueRemove(values[values.length - 1]);
                  }
                }}
              />
            </Combobox.EventsTarget>
          </Pill.Group>
        </PillsInput>
      </Combobox.DropdownTarget>

      <Combobox.Dropdown>
        <Combobox.Options>
          {options.map((value) => {
            return (
              <Combobox.Option
                value={value}
                key={value}
                active={values.includes(value)}
              >
                <Group gap="sm">
                  {values.includes(value) ? (
                    <CheckIcon size={12} color="gray" />
                  ) : null}
                  <StatePill
                    value={
                      optionCount ? `${value} (${optionCount(value)})` : value
                    }
                    className={optionClass(value)}
                  />
                </Group>
              </Combobox.Option>
            );
          })}
        </Combobox.Options>
      </Combobox.Dropdown>
    </Combobox>
  );
};
