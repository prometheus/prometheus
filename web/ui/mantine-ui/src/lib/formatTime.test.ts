import {
  humanizeDuration,
  humanizeDurationRelative,
  formatPrometheusDuration,
  parsePrometheusDuration,
  parseTime,
} from "./formatTime";

describe("parsePrometheusDuration", () => {
  test("parses Prometheus duration strings", () => {
    expect(parsePrometheusDuration("0")).toBe(0);
    expect(parsePrometheusDuration("5m")).toBe(5 * 60 * 1000);
    expect(parsePrometheusDuration("1h30m")).toBe(90 * 60 * 1000);
    expect(parsePrometheusDuration("2d3h4m5s6ms")).toBe(
      2 * 24 * 60 * 60 * 1000 +
        3 * 60 * 60 * 1000 +
        4 * 60 * 1000 +
        5 * 1000 +
        6
    );
  });

  test("returns null for empty or invalid durations", () => {
    expect(parsePrometheusDuration("")).toBeNull();
    expect(parsePrometheusDuration("1.5h")).toBeNull();
    expect(parsePrometheusDuration("5m1h")).toBeNull();
    expect(parsePrometheusDuration("10q")).toBeNull();
  });
});

describe("formatPrometheusDuration", () => {
  test('returns "0s" for 0 milliseconds', () => {
    expect(formatPrometheusDuration(0)).toBe("0s");
  });

  test("formats milliseconds correctly", () => {
    expect(formatPrometheusDuration(1)).toBe("1ms");
    expect(formatPrometheusDuration(999)).toBe("999ms");
  });

  test("formats seconds correctly", () => {
    expect(formatPrometheusDuration(1000)).toBe("1s");
    expect(formatPrometheusDuration(1500)).toBe("1s500ms");
    expect(formatPrometheusDuration(59999)).toBe("59s999ms");
  });

  test("formats minutes correctly", () => {
    expect(formatPrometheusDuration(60000)).toBe("1m");
    expect(formatPrometheusDuration(120000)).toBe("2m");
    expect(formatPrometheusDuration(3599999)).toBe("59m59s999ms");
  });

  test("formats hours correctly", () => {
    expect(formatPrometheusDuration(3600000)).toBe("1h");
    expect(formatPrometheusDuration(7200000)).toBe("2h");
    expect(formatPrometheusDuration(86399999)).toBe("23h59m59s999ms");
  });

  test("formats days correctly", () => {
    expect(formatPrometheusDuration(86400000)).toBe("1d");
    expect(formatPrometheusDuration(172800000)).toBe("2d");
    expect(formatPrometheusDuration(86400000 * 365 - 1)).toBe(
      "364d23h59m59s999ms"
    );
  });

  test("handles negative durations", () => {
    expect(formatPrometheusDuration(-1000)).toBe("-1s");
    expect(formatPrometheusDuration(-86400000)).toBe("-1d");
  });

  test("combines multiple units correctly", () => {
    expect(
      formatPrometheusDuration(86400000 + 3600000 + 60000 + 1000 + 1)
    ).toBe("1d1h1m1s1ms");
  });

  test("omits zero values", () => {
    expect(formatPrometheusDuration(86400000 + 1000)).toBe("1d1s");
  });
});

describe("humanizeDuration", () => {
  test('returns "0s" for 0 milliseconds', () => {
    expect(humanizeDuration(0)).toBe("0s");
  });

  test("formats submilliseconds correctly", () => {
    expect(humanizeDuration(0.1)).toBe("0ms");
    expect(humanizeDuration(0.6)).toBe("1ms");
    expect(humanizeDuration(0.000001)).toBe("0ms");
  });

  test("formats milliseconds correctly", () => {
    expect(humanizeDuration(1)).toBe("1ms");
    expect(humanizeDuration(999)).toBe("999ms");
  });

  test("formats seconds correctly", () => {
    expect(humanizeDuration(1000)).toBe("1s");
    expect(humanizeDuration(1500)).toBe("1.5s");
    expect(humanizeDuration(59999)).toBe("59.999s");
  });

  test("formats minutes correctly", () => {
    expect(humanizeDuration(60000)).toBe("1m");
    expect(humanizeDuration(120000)).toBe("2m");
    expect(humanizeDuration(3599999)).toBe("59m 59.999s");
  });

  test("formats hours correctly", () => {
    expect(humanizeDuration(3600000)).toBe("1h");
    expect(humanizeDuration(7200000)).toBe("2h");
    expect(humanizeDuration(86399999)).toBe("23h 59m 59.999s");
  });

  test("formats days correctly", () => {
    expect(humanizeDuration(86400000)).toBe("1d");
    expect(humanizeDuration(172800000)).toBe("2d");
    expect(humanizeDuration(86400000 * 365 - 1)).toBe("364d 23h 59m 59.999s");
    expect(humanizeDuration(86400000 * 365 - 1)).toBe("364d 23h 59m 59.999s");
  });

  test("handles negative durations", () => {
    expect(humanizeDuration(-1000)).toBe("-1s");
    expect(humanizeDuration(-86400000)).toBe("-1d");
  });

  test("combines multiple units correctly", () => {
    expect(humanizeDuration(86400000 + 3600000 + 60000 + 1000 + 1)).toBe(
      "1d 1h 1m 1.001s"
    );
  });

  test("omits zero values", () => {
    expect(humanizeDuration(86400000 + 1000)).toBe("1d 1s");
  });
});

describe("parseTime", () => {
  test("parses date and time strings", () => {
    expect(parseTime("2024-01-15 12:30:45")).toBe(1705321845000);
    expect(parseTime("2024-01-01 00:00:00")).toBe(1704067200000);
  });

  test("parses ISO 8601 UTC strings", () => {
    expect(parseTime("2024-01-15T12:30:45Z")).toBe(1705321845000);
    expect(parseTime("1970-01-01T00:00:00Z")).toBe(0);
  });

  test("parses negative timestamps", () => {
    expect(parseTime("0001-01-01T00:00:00Z")).toBe(-62135596800000);
    expect(parseTime("1969-12-31T23:59:59Z")).toBe(-1000);
  });

  test("returns null for invalid date strings", () => {
    expect(parseTime("invalid")).toBeNull();
    expect(parseTime("")).toBeNull();
    expect(parseTime("not-a-date")).toBeNull();
  });
});

describe("humanizeDurationRelative", () => {
  test("formats relative duration for valid timestamps", () => {
    const start = "2024-01-15 12:30:00";
    const end = 1705321860000; // 60s later (2024-01-15 12:31:00 UTC)
    expect(humanizeDurationRelative(start, end)).toBe("1m ago");
  });

  test("respects custom suffix", () => {
    const start = "2024-01-15 12:30:00";
    const end = 1705321860000;
    expect(humanizeDurationRelative(start, end, "")).toBe("1m");
    expect(humanizeDurationRelative(start, end, " earlier")).toBe("1m earlier");
  });

  test("handles negative elapsed time (future timestamps)", () => {
    const start = "2024-01-15 12:31:00";
    const end = 1705321800000; // 60s earlier
    expect(humanizeDurationRelative(start, end)).toBe("-1m ago");
  });

  test('returns "never" for negative timestamps', () => {
    expect(
      humanizeDurationRelative("0001-01-01T00:00:00Z", 1705321845000),
    ).toBe("never");
    expect(
      humanizeDurationRelative("1969-12-31T23:59:59Z", 1705321845000),
    ).toBe("never");
  });

  test('returns "never" for invalid timestamp strings', () => {
    expect(humanizeDurationRelative("invalid", 1705321845000)).toBe("never");
    expect(humanizeDurationRelative("", 1705321845000)).toBe("never");
    expect(humanizeDurationRelative("not-a-date", 1705321845000)).toBe("never");
  });

  test('returns "never" for non-finite end values', () => {
    expect(humanizeDurationRelative("2024-01-15 12:30:00", NaN)).toBe("never");
    expect(humanizeDurationRelative("2024-01-15 12:30:00", Infinity)).toBe(
      "never",
    );
    expect(humanizeDurationRelative("2024-01-15 12:30:00", -Infinity)).toBe(
      "never",
    );
  });

  test('returns "never" when both start is invalid and end is non-finite', () => {
    expect(humanizeDurationRelative("invalid", NaN)).toBe("never");
  });
});
