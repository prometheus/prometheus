import { useState } from "react";
import {
  Table,
  UnstyledButton,
  Group,
  Text,
  Center,
  TextInput,
  rem,
  keys,
} from "@mantine/core";
import {
  IconSelector,
  IconChevronDown,
  IconChevronUp,
  IconSearch,
} from "@tabler/icons-react";
import classes from "./FlagsPage.module.css";
import { useSuspenseAPIQuery } from "../api/api";
import InfoPageStack from "../components/InfoPageStack";
import InfoPageCard from "../components/InfoPageCard";
import { inputIconStyle } from "../styles";

interface RowData {
  flag: string;
  value: string;
}

interface ThProps {
  children: React.ReactNode;
  reversed: boolean;
  sorted: boolean;
  onSort(): void;
}

function Th({ children, reversed, sorted, onSort }: ThProps) {
  const Icon = sorted
    ? reversed
      ? IconChevronUp
      : IconChevronDown
    : IconSelector;
  return (
    <Table.Th className={classes.th}>
      <UnstyledButton onClick={onSort} className={classes.control}>
        <Group justify="space-between">
          <Text fw={600} fz="sm">
            {children}
          </Text>
          <Center className={classes.icon}>
            <Icon style={{ width: rem(16), height: rem(16) }} stroke={1.5} />
          </Center>
        </Group>
      </UnstyledButton>
    </Table.Th>
  );
}

function filterData(data: RowData[], search: string) {
  const query = search.toLowerCase().trim();
  return data.filter((item) =>
    keys(data[0]).some((key) => item[key].toLowerCase().includes(query))
  );
}

function sortData(
  data: RowData[],
  payload: { sortBy: keyof RowData | null; reversed: boolean; search: string }
) {
  const { sortBy } = payload;

  if (!sortBy) {
    return filterData(data, payload.search);
  }

  return filterData(
    [...data].sort((a, b) => {
      if (payload.reversed) {
        return b[sortBy].localeCompare(a[sortBy]);
      }

      return a[sortBy].localeCompare(b[sortBy]);
    }),
    payload.search
  );
}

export default function FlagsPage() {
  const { data } = useSuspenseAPIQuery<Record<string, string>>({
    path: `/status/flags`,
  });

  const flags = Object.entries(data.data).map(([flag, value]) => ({
    flag,
    value,
  }));

  const [search, setSearch] = useState("");
  const [sortedData, setSortedData] = useState(flags);
  const [sortBy, setSortBy] = useState<keyof RowData | null>(null);
  const [reverseSortDirection, setReverseSortDirection] = useState(false);

  const setSorting = (field: keyof RowData) => {
    const reversed = field === sortBy ? !reverseSortDirection : false;
    setReverseSortDirection(reversed);
    setSortBy(field);
    setSortedData(sortData(flags, { sortBy: field, reversed, search }));
  };

  const handleSearchChange = (event: React.ChangeEvent<HTMLInputElement>) => {
    const { value } = event.currentTarget;
    setSearch(value);
    setSortedData(
      sortData(flags, { sortBy, reversed: reverseSortDirection, search: value })
    );
  };

  const rows = sortedData.map((row) => (
    <Table.Tr key={row.flag}>
      <Table.Td>
        <code>--{row.flag}</code>
      </Table.Td>
      <Table.Td>
        <code>{row.value}</code>
      </Table.Td>
    </Table.Tr>
  ));

  return (
    <InfoPageStack>
      <InfoPageCard>
        <TextInput
          placeholder="Filter by flag name or value"
          mb="md"
          autoFocus
          leftSection={<IconSearch style={inputIconStyle} />}
          value={search}
          onChange={handleSearchChange}
        />
        <Table
          horizontalSpacing="md"
          verticalSpacing="xs"
          miw={700}
          layout="fixed"
        >
          <Table.Tbody>
            <Table.Tr>
              <Th
                sorted={sortBy === "flag"}
                reversed={reverseSortDirection}
                onSort={() => setSorting("flag")}
              >
                Flag
              </Th>

              <Th
                sorted={sortBy === "value"}
                reversed={reverseSortDirection}
                onSort={() => setSorting("value")}
              >
                Value
              </Th>
            </Table.Tr>
          </Table.Tbody>
          <Table.Tbody>
            {rows.length > 0 ? (
              rows
            ) : (
              <Table.Tr>
                <Table.Td colSpan={2}>
                  <Text fw={500} ta="center">
                    Nothing found
                  </Text>
                </Table.Td>
              </Table.Tr>
            )}
          </Table.Tbody>
        </Table>
      </InfoPageCard>
    </InfoPageStack>
  );
}
