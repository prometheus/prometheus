import dayjs from "dayjs";
import duration from "dayjs/plugin/duration";
dayjs.extend(duration);
import utc from "dayjs/plugin/utc";
dayjs.extend(utc);

// Parse Prometheus-specific duration strings such as "5m" or "1d2h3m4s" into milliseconds.
export const parsePrometheusDuration = (durationStr: string): number | null => {
  if (durationStr === "") {
    return null;
  }
  if (durationStr === "0") {
    // Allow 0 without a unit.
    return 0;
  }

  const durationRE = new RegExp(
    "^(([0-9]+)y)?(([0-9]+)w)?(([0-9]+)d)?(([0-9]+)h)?(([0-9]+)m)?(([0-9]+)s)?(([0-9]+)ms)?$"
  );
  const matches = durationStr.match(durationRE);
  if (!matches) {
    return null;
  }

  let dur = 0;

  // Parse the match at pos `pos` in the regex and use `mult` to turn that
  // into ms, then add that value to the total parsed duration.
  const m = (pos: number, mult: number) => {
    if (matches[pos] === undefined) {
      return;
    }
    const n = parseInt(matches[pos]);
    dur += n * mult;
  };

  m(2, 1000 * 60 * 60 * 24 * 365); // y
  m(4, 1000 * 60 * 60 * 24 * 7); // w
  m(6, 1000 * 60 * 60 * 24); // d
  m(8, 1000 * 60 * 60); // h
  m(10, 1000 * 60); // m
  m(12, 1000); // s
  m(14, 1); // ms

  return dur;
};

// Used by:
// - formatPrometheusDuration() => "5d5m2s123ms"
// - humanizeDuration()         => "5d 5m 2.123s"
const formatDuration = (
  d: number,
  componentSeparator?: string,
  showFractionalSeconds?: boolean
): string => {
  if (d === 0) {
    return "0s";
  }

  const sign = d < 0 ? "-" : "";
  let ms = Math.abs(d);
  const r: string[] = [];

  for (const { unit, mult, exact } of [
    // Only format years and weeks if the remainder is zero, as it is often
    // easier to read 90d than 12w6d.
    { unit: "y", mult: 1000 * 60 * 60 * 24 * 365, exact: true },
    { unit: "w", mult: 1000 * 60 * 60 * 24 * 7, exact: true },
    { unit: "d", mult: 1000 * 60 * 60 * 24, exact: false },
    { unit: "h", mult: 1000 * 60 * 60, exact: false },
    { unit: "m", mult: 1000 * 60, exact: false },
    { unit: "s", mult: 1000, exact: false },
    { unit: "ms", mult: 1, exact: false },
  ]) {
    if (exact && ms % mult !== 0) {
      continue;
    }
    const v = Math.floor(ms / mult);
    if (v > 0) {
      ms -= v * mult;
      if (showFractionalSeconds && unit === "s" && ms > 0) {
        // Show "2.34s" instead of "2s 340ms".
        r.push(`${parseFloat((v + ms / 1000).toFixed(3))}s`);
        break;
      } else {
        r.push(`${v}${unit}`);
      }
    }
    if (r.length == 0 && unit == "ms") {
        r.push(`${Math.round(ms)}ms`)
    }
  }

  return sign + r.join(componentSeparator || "");
};

// Format a duration in milliseconds into a Prometheus duration string like "1d2h3m4s".
export const formatPrometheusDuration = (d: number): string => {
  return formatDuration(d);
};

export function parseTime(timeText: string): number {
  return dayjs.utc(timeText).valueOf();
}

export const now = (): number => dayjs().valueOf();

export const humanizeDuration = (milliseconds: number): string => {
  return formatDuration(milliseconds, " ", true);
};

export const humanizeDurationRelative = (
  startStr: string,
  end: number,
  suffix: string = " ago"
): string => {
  const start = parseTime(startStr);
  if (start < 0) {
    return "never";
  }
  return humanizeDuration(end - start) + suffix;
};

export const formatTimestamp = (t: number, useLocalTime: boolean) =>
  useLocalTime
    ? dayjs.unix(t).tz(dayjs.tz.guess()).format()
    : dayjs.unix(t).utc().format();
