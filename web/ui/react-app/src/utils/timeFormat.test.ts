import { formatTime, parseTime, formatRange, parseRange, humanizeDuration, formatRelative, now } from './timeFormat';

describe('formatTime', () => {
  it('returns a time string representing the time in seconds', () => {
    expect(formatTime(1572049380000)).toEqual('2019-10-26 00:23');
    expect(formatTime(0)).toEqual('1970-01-01 00:00');
  });
});

describe('parseTime', () => {
  it('returns a time string representing the time in seconds', () => {
    expect(parseTime('2019-10-26 00:23')).toEqual(1572049380000);
    expect(parseTime('1970-01-01 00:00')).toEqual(0);
    expect(parseTime('0001-01-01T00:00:00Z')).toEqual(-62135596800000);
  });
});

describe('formatRange', () => {
  it('returns a time string representing the time in seconds in one unit', () => {
    expect(formatRange(60 * 60 * 24 * 365)).toEqual('1y');
    expect(formatRange(60 * 60 * 24 * 7)).toEqual('1w');
    expect(formatRange(2 * 60 * 60 * 24)).toEqual('2d');
    expect(formatRange(60 * 60)).toEqual('1h');
    expect(formatRange(7 * 60)).toEqual('7m');
    expect(formatRange(63)).toEqual('63s');
  });
});

describe('parseRange', () => {
  it('returns a time string representing the time in seconds in one unit', () => {
    expect(parseRange('1y')).toEqual(60 * 60 * 24 * 365);
    expect(parseRange('1w')).toEqual(60 * 60 * 24 * 7);
    expect(parseRange('2d')).toEqual(2 * 60 * 60 * 24);
    expect(parseRange('1h')).toEqual(60 * 60);
    expect(parseRange('7m')).toEqual(7 * 60);
    expect(parseRange('63s')).toEqual(63);
  });
});

describe('humanizeDuration', () => {
  it('humanizes zero', () => {
    expect(humanizeDuration(0)).toEqual('0s');
  });
  it('humanizes milliseconds', () => {
    expect(humanizeDuration(1.234567)).toEqual('1.235ms');
    expect(humanizeDuration(12.34567)).toEqual('12.346ms');
    expect(humanizeDuration(123.45678)).toEqual('123.457ms');
    expect(humanizeDuration(123)).toEqual('123.000ms');
  });
  it('humanizes seconds', () => {
    expect(humanizeDuration(12340)).toEqual('12.340s');
  });
  it('humanizes minutes', () => {
    expect(humanizeDuration(1234567)).toEqual('20m 34s');
  });

  it('humanizes hours', () => {
    expect(humanizeDuration(12345678)).toEqual('3h 25m 45s');
  });

  it('humanizes days', () => {
    expect(humanizeDuration(123456789)).toEqual('1d 10h 17m 36s');
    expect(humanizeDuration(123456789000)).toEqual('1428d 21h 33m 9s');
  });
  it('takes sign into account', () => {
    expect(humanizeDuration(-123456789000)).toEqual('-1428d 21h 33m 9s');
  });
});

describe('formatRelative', () => {
  it('renders never for pre-beginning-of-time strings', () => {
    expect(formatRelative('0001-01-01T00:00:00Z', now())).toEqual('Never');
  });
  it('renders a humanized duration for sane durations', () => {
    expect(formatRelative('2019-11-04T09:15:29.578701-07:00', parseTime('2019-11-04T09:15:35.8701-07:00'))).toEqual(
      '6.292s'
    );
    expect(formatRelative('2019-11-04T09:15:35.8701-07:00', parseTime('2019-11-04T09:15:29.578701-07:00'))).toEqual(
      '-6.292s'
    );
  });
});
