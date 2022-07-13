import { formatValue, parseValue, getOptions } from './GraphHelpers';
import moment from 'moment';
require('../../vendor/flot/jquery.flot'); // need for $.colors

describe('GraphHelpers', () => {
  describe('formatValue', () => {
    it('formats tick values correctly', () => {
      [
        { input: null, output: 'null' },
        { input: 0, output: '0.00' },
        { input: 2e24, output: '2.00Y' },
        { input: 2e23, output: '200.00Z' },
        { input: 2e22, output: '20.00Z' },
        { input: 2e21, output: '2.00Z' },
        { input: 2e19, output: '20.00E' },
        { input: 2e18, output: '2.00E' },
        { input: 2e17, output: '200.00P' },
        { input: 2e16, output: '20.00P' },
        { input: 2e15, output: '2.00P' },
        { input: 1e15, output: '1.00P' },
        { input: 2e14, output: '200.00T' },
        { input: 2e13, output: '20.00T' },
        { input: 2e12, output: '2.00T' },
        { input: 2e11, output: '200.00G' },
        { input: 2e10, output: '20.00G' },
        { input: 2e9, output: '2.00G' },
        { input: 2e8, output: '200.00M' },
        { input: 2e7, output: '20.00M' },
        { input: 2e6, output: '2.00M' },
        { input: 2e5, output: '200.00k' },
        { input: 2e4, output: '20.00k' },
        { input: 2e3, output: '2.00k' },
        { input: 2e2, output: '200.00' },
        { input: 2e1, output: '20.00' },
        { input: 2, output: '2.00' },
        { input: 2e-1, output: '0.20' },
        { input: 2e-2, output: '0.02' },
        { input: 2e-3, output: '2.00m' },
        { input: 2e-4, output: '0.20m' },
        { input: 2e-5, output: '0.02m' },
        { input: 2e-6, output: '2.00µ' },
        { input: 2e-7, output: '0.20µ' },
        { input: 2e-8, output: '0.02µ' },
        { input: 2e-9, output: '2.00n' },
        { input: 2e-10, output: '0.20n' },
        { input: 2e-11, output: '0.02n' },
        { input: 2e-12, output: '2.00p' },
        { input: 2e-13, output: '0.20p' },
        { input: 2e-14, output: '0.02p' },
        { input: 2e-15, output: '2.00f' },
        { input: 2e-16, output: '0.20f' },
        { input: 2e-17, output: '0.02f' },
        { input: 2e-18, output: '2.00a' },
        { input: 2e-19, output: '0.20a' },
        { input: 2e-20, output: '0.02a' },
        { input: 1e-21, output: '1.00z' },
        { input: 2e-21, output: '2.00z' },
        { input: 2e-22, output: '0.20z' },
        { input: 2e-23, output: '0.02z' },
        { input: 2e-24, output: '2.00y' },
        { input: 2e-25, output: '0.20y' },
        { input: 2e-26, output: '0.02y' },
      ].map((t) => {
        expect(formatValue(t.input)).toBe(t.output);
      });
    });
    it('should throw error if no match', () => {
      expect(() => formatValue(undefined as any)).toThrowError("couldn't format a value, this is a bug");
    });
  });
  describe('parseValue', () => {
    it('should parse number properly', () => {
      expect(parseValue('12.3e')).toEqual(12.3);
    });
    it('should return 0 if value is NaN and stacked prop is true', () => {
      expect(parseValue('asd')).toEqual(null);
    });
    it('should return null if value is NaN and stacked prop is false', () => {
      expect(parseValue('asd')).toBeNull();
    });
  });
  describe('Plot options', () => {
    it('should configure options properly if stacked prop is true', () => {
      expect(getOptions(true, false)).toMatchObject({
        series: {
          stack: false,
          lines: { lineWidth: 1, steps: false, fill: true },
          shadowSize: 0,
        },
      });
    });
    it('should configure options properly if stacked prop is false', () => {
      expect(getOptions(false, false)).toMatchObject({
        series: {
          stack: false,
          lines: { lineWidth: 2, steps: false, fill: false },
          shadowSize: 0,
        },
      });
    });
    it('should configure options properly if useLocalTime prop is true', () => {
      expect(getOptions(true, true)).toMatchObject({
        xaxis: {
          mode: 'time',
          showTicks: true,
          showMinorTicks: true,
          timeBase: 'milliseconds',
          timezone: 'browser',
        },
      });
    });
    it('should configure options properly if useLocalTime prop is false', () => {
      expect(getOptions(false, false)).toMatchObject({
        xaxis: {
          mode: 'time',
          showTicks: true,
          showMinorTicks: true,
          timeBase: 'milliseconds',
        },
      });
    });
    it('should return proper tooltip html from options', () => {
      expect(
        getOptions(true, false).tooltip.content('', 1572128592, 1572128592, {
          series: { labels: { foo: '1', bar: '2' }, color: '' },
        } as any)
      ).toEqual(
        `
            <div class="date">1970-01-19 04:42:08 +00:00</div>
            <div>
              <span class="detail-swatch" style="background-color: "></span>
              <span>value: <strong>1572128592</strong></span>
            </div>
            <div class="mt-2 mb-1 font-weight-bold">Series:</div>
            
            <div class="labels">
              
              
              <div class="mb-1"><strong>foo</strong>: 1</div><div class="mb-1"><strong>bar</strong>: 2</div>
            </div>`
      );
    });
    it('should return proper tooltip html from options with local time', () => {
      moment.tz.setDefault('America/New_York');
      expect(
        getOptions(true, true).tooltip.content('', 1572128592, 1572128592, {
          series: { labels: { foo: '1', bar: '2' }, color: '' },
        } as any)
      ).toEqual(`
            <div class="date">1970-01-18 23:42:08 -05:00</div>
            <div>
              <span class="detail-swatch" style="background-color: "></span>
              <span>value: <strong>1572128592</strong></span>
            </div>
            <div class="mt-2 mb-1 font-weight-bold">Series:</div>
            
            <div class="labels">
              
              
              <div class="mb-1"><strong>foo</strong>: 1</div><div class="mb-1"><strong>bar</strong>: 2</div>
            </div>`);
    });
    it('should return proper tooltip for exemplar', () => {
      expect(
        getOptions(true, false).tooltip.content('', 1572128592, 1572128592, {
          series: { labels: { foo: '1', bar: '2' }, seriesLabels: { foo: '2', bar: '3' }, color: '' },
        } as any)
      ).toEqual(`
            <div class="date">1970-01-19 04:42:08 +00:00</div>
            <div>
              <span class="detail-swatch" style="background-color: "></span>
              <span>value: <strong>1572128592</strong></span>
            </div>
            <div class="mt-2 mb-1 font-weight-bold">Trace exemplar:</div>
            
            <div class="labels">
              
              
              <div class="mb-1"><strong>foo</strong>: 1</div><div class="mb-1"><strong>bar</strong>: 2</div>
            </div>
            
            <div class="mt-2 mb-1 font-weight-bold">Associated series:</div>
            <div class="labels">
              
              
              <div class="mb-1"><strong>foo</strong>: 2</div><div class="mb-1"><strong>bar</strong>: 3</div>
            </div>`);
    });
    it('should render Plot with proper options', () => {
      expect(getOptions(true, false)).toEqual({
        grid: {
          hoverable: true,
          clickable: true,
          autoHighlight: true,
          mouseActiveRadius: 100,
        },
        legend: { show: false },
        xaxis: {
          mode: 'time',
          showTicks: true,
          showMinorTicks: true,
          timeBase: 'milliseconds',
        },
        yaxis: { tickFormatter: expect.anything() },
        crosshair: { mode: 'xy', color: '#bbb' },
        tooltip: {
          show: true,
          cssClass: 'graph-tooltip',
          content: expect.anything(),
          defaultTheme: false,
          lines: true,
        },
        series: {
          stack: false,
          lines: { lineWidth: 1, steps: false, fill: true },
          shadowSize: 0,
        },
        selection: {
          mode: 'x',
        },
      });
    });
  });
});
