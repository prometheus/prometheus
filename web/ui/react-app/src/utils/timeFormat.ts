import moment from 'moment-timezone';

const rangeUnits: { [unit: string]: number } = {
  y: 60 * 60 * 24 * 365,
  w: 60 * 60 * 24 * 7,
  d: 60 * 60 * 24,
  h: 60 * 60,
  m: 60,
  s: 1,
};

export function parseRange(rangeText: string): number | null {
  const rangeRE = new RegExp('^([0-9]+)([ywdhms]+)$');
  const matches = rangeText.match(rangeRE);
  if (!matches || matches.length !== 3) {
    return null;
  }
  const value = parseInt(matches[1]);
  const unit = matches[2];
  return value * rangeUnits[unit];
}

export function formatRange(range: number): string {
  for (const unit of Object.keys(rangeUnits)) {
    if (range % rangeUnits[unit] === 0) {
      return range / rangeUnits[unit] + unit;
    }
  }
  return range + 's';
}

export function parseTime(timeText: string): number {
  return moment.utc(timeText).valueOf();
}

export function formatTime(time: number): string {
  return moment.utc(time).format('YYYY-MM-DD HH:mm');
}

export const now = (): number => moment().valueOf();

export const humanizeDuration = (milliseconds: number): string => {
  const sign = milliseconds < 0 ? '-' : '';
  const unsignedMillis = milliseconds < 0 ? -1 * milliseconds : milliseconds;
  const duration = moment.duration(unsignedMillis, 'ms');
  const ms = Math.floor(duration.milliseconds());
  const s = Math.floor(duration.seconds());
  const m = Math.floor(duration.minutes());
  const h = Math.floor(duration.hours());
  const d = Math.floor(duration.asDays());
  if (d !== 0) {
    return `${sign}${d}d ${h}h ${m}m ${s}s`;
  }
  if (h !== 0) {
    return `${sign}${h}h ${m}m ${s}s`;
  }
  if (m !== 0) {
    return `${sign}${m}m ${s}s`;
  }
  if (s !== 0) {
    return `${sign}${s}.${ms}s`;
  }
  if (unsignedMillis > 0) {
    return `${sign}${unsignedMillis.toFixed(3)}ms`;
  }
  return '0s';
};

export const formatRelative = (startStr: string, end: number): string => {
  const start = parseTime(startStr);
  if (start < 0) {
    return 'Never';
  }
  return humanizeDuration(end - start);
};
