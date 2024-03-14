import { useState } from "react";
import {
  Badge,
  CheckIcon,
  CloseButton,
  Combobox,
  ComboboxChevron,
  ComboboxClearButton,
  Group,
  Input,
  Pill,
  PillGroup,
  PillsInput,
  useCombobox,
} from "@mantine/core";
import badgeClasses from "../Badge.module.css";
import { IconFilter } from "@tabler/icons-react";
import { IconFilterSearch } from "@tabler/icons-react";

interface StatePillProps extends React.ComponentPropsWithoutRef<"div"> {
  value: string;
  onRemove?: () => void;
}

export function StatePill({ value, onRemove, ...others }: StatePillProps) {
  return (
    <Pill
      fw={600}
      style={{ textTransform: "uppercase", fontWeight: 600 }}
      className={
        value === "inactive"
          ? badgeClasses.healthOk
          : value === "pending"
          ? badgeClasses.healthWarn
          : badgeClasses.healthErr
      }
      onRemove={onRemove}
      {...others}
      withRemoveButton={!!onRemove}
    >
      {value}
    </Pill>
  );
}

export function AlertStateMultiSelect() {
  const combobox = useCombobox({
    onDropdownClose: () => combobox.resetSelectedOption(),
    onDropdownOpen: () => combobox.updateSelectedOptionIndex("active"),
  });

  const [values, setValues] = useState<string[]>([]);

  const handleValueSelect = (val: string) =>
    setValues((current) =>
      current.includes(val)
        ? current.filter((v) => v !== val)
        : [...current, val]
    );

  const handleValueRemove = (val: string) =>
    setValues((current) => current.filter((v) => v !== val));

  const renderedValues = values.map((item) => (
    <StatePill
      value={item}
      onRemove={() => handleValueRemove(item)}
      key={item}
    />
  ));

  const options = ["inactive", "pending", "firing"].map((value) => {
    return (
      <Combobox.Option
        value={value}
        key={value}
        active={values.includes(value)}
      >
        <Group gap="sm">
          {values.includes(value) ? <CheckIcon size={12} color="gray" /> : null}
          <StatePill value={value} />
        </Group>
      </Combobox.Option>
    );
  });

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
          leftSection={<IconFilter size={14} />}
          rightSection={
            values.length > 0 ? (
              <ComboboxClearButton onClear={() => setValues([])} />
            ) : (
              <ComboboxChevron />
            )
          }
        >
          <Pill.Group>
            {renderedValues.length > 0 ? (
              renderedValues
            ) : (
              <Input.Placeholder>Filter by alert state</Input.Placeholder>
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
        <Combobox.Options>{options}</Combobox.Options>
      </Combobox.Dropdown>
    </Combobox>
  );
}
