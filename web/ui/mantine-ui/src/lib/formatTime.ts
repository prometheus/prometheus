import dayjs from "dayjs";
import duration from "dayjs/plugin/duration";
dayjs.extend(duration);
import utc from "dayjs/plugin/utc";
dayjs.extend(utc);

export const parseDuration = (durationStr: string): number | null => {
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

export const formatDuration = (d: number): string => {
  let ms = d;
  let r = "";
  if (ms === 0) {
    return "0s";
  }

  const f = (unit: string, mult: number, exact: boolean) => {
    if (exact && ms % mult !== 0) {
      return;
    }
    const v = Math.floor(ms / mult);
    if (v > 0) {
      r += `${v}${unit}`;
      ms -= v * mult;
    }
  };

  // Only format years and weeks if the remainder is zero, as it is often
  // easier to read 90d than 12w6d.
  f("y", 1000 * 60 * 60 * 24 * 365, true);
  f("w", 1000 * 60 * 60 * 24 * 7, true);

  f("d", 1000 * 60 * 60 * 24, false);
  f("h", 1000 * 60 * 60, false);
  f("m", 1000 * 60, false);
  f("s", 1000, false);
  f("ms", 1, false);

  return r;
};

export function parseTime(timeText: string): number {
  return dayjs.utc(timeText).valueOf();
}

export const now = (): number => dayjs().valueOf();

export const humanizeDuration = (milliseconds: number): string => {
  if (milliseconds === 0) {
    return "0s";
  }

  const duration = dayjs.duration(Math.abs(milliseconds), "ms");
  const ms = Math.floor(duration.milliseconds());
  const s = Math.floor(duration.seconds());
  const m = Math.floor(duration.minutes());
  const h = Math.floor(duration.hours());
  const d = Math.floor(duration.asDays());
  const parts: string[] = [];
  if (d !== 0) {
    parts.push(`${d}d`);
  }
  if (h !== 0) {
    parts.push(`${h}h`);
  }
  if (m !== 0) {
    parts.push(`${m}m`);
  }
  if (s !== 0) {
    parts.push(`${s}s`);
  }
  if (ms !== 0) {
    parts.push(`${ms}ms`);
  }
  return (milliseconds < 0 ? "-" : "") + parts.join(" ");
};

export const formatRelative = (
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
