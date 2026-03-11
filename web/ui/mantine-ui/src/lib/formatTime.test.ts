import { humanizeDuration, formatPrometheusDuration } from "./formatTime";

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
