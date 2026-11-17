import uPlot from "uplot";
import { applySeriesVisibility } from "./uPlotChartHelpers";

describe("applySeriesVisibility", () => {
  const series = (label: string): uPlot.Series => ({ label, show: true });

  const cases: {
    name: string;
    series: uPlot.Series[];
    saved: Map<string, boolean>;
    expectedShow: (boolean | undefined)[];
  }[] = [
    {
      name: "leaves series untouched when nothing was saved for them",
      series: [{}, series("up"), series("down")],
      saved: new Map(),
      expectedShow: [undefined, true, true],
    },
    {
      name: "restores a hidden series from the saved map",
      series: [{}, series("up"), series("down")],
      saved: new Map([["down", false]]),
      expectedShow: [undefined, true, false],
    },
    {
      name: "ignores saved labels that no longer have a matching series",
      series: [{}, series("up")],
      saved: new Map([["down", false]]),
      expectedShow: [undefined, true],
    },
    {
      name: "skips the leading x-axis placeholder, which has no label",
      series: [{}, series("up")],
      saved: new Map([["up", false]]),
      expectedShow: [undefined, false],
    },
  ];

  for (const c of cases) {
    test(c.name, () => {
      const result = applySeriesVisibility(c.series, c.saved);
      expect(result.map((s) => s.show)).toEqual(c.expectedShow);
    });
  }

  test("does not mutate the input series", () => {
    const input = [series("up")];
    applySeriesVisibility(input, new Map([["up", false]]));
    expect(input[0].show).toBe(true);
  });
});
