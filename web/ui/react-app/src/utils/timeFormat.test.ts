import { formatTime, parseTime, formatRange, parseRange } from './timeFormat';

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
