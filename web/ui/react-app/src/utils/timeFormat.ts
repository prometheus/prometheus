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
