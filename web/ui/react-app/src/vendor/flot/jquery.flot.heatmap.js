import moment from 'moment-timezone';
import {formatValue} from "../../pages/graph/GraphHelpers";

(function ($) {
  let mouseMoveHandler = null;

  function init(plot) {
    const fillPalette = generateGradient("#F7C5B9", "#E6522C", "#52180A");

    plot.hooks.draw.push((plot, ctx) => {
      const options = plot.getOptions();
      if (!options.series.heatmap) return

      const series = plot.getData();
      const fills = countsToFills(series.flatMap(s => s.data.map(d => d[1])), fillPalette);
      series.forEach((s, i) => drawHeatmap(s, plot, ctx, i, fills));
    });

    plot.hooks.bindEvents.push((plot, eventHolder) => {
      const options = plot.getOptions();
      if (!options.series.heatmap || !options.tooltip.show) return

      mouseMoveHandler = (e) => {
        removeTooltip();
        const { left: xOffset, top: yOffset } = plot.offset();
        const pos = plot.c2p({ left: e.pageX - xOffset, top: e.pageY - yOffset });
        const seriesIdx = Math.floor(pos.y);
        const series = plot.getData();

        for (let i = 0; i < series.length; i++) {
          if (seriesIdx !== i) continue;

          const s = series[i];
          for (let j = 0; j < s.data.length - 1; j++) {
            const [xStartVal, yStartVal] = s.data[j];
            const [xEndVal] = s.data[j + 1];
            const isIncluded = pos.x >= xStartVal && pos.x <= xEndVal
            if (yStartVal && isIncluded) {
              showTooltip({
                cssClass: options.tooltip.cssClass,
                x: e.pageX,
                y: e.pageY,
                value: formatValue(yStartVal),
                dateTime: [xStartVal, xEndVal].map(t => moment(t).format('YYYY-MM-DD HH:mm:ss Z')),
                label: `{ ${Object.entries(s.labels).map(([key, val]) => `${key}: ${val}`).join(',\n')} }`,
              });

              break;
            }
          }
        }
      }

      $(eventHolder).bind('mousemove', mouseMoveHandler);
    });

    plot.hooks.shutdown.push(function (plot, eventHolder) {
      removeTooltip();
      $(eventHolder).unbind("mousemove", mouseMoveHandler);
    });
  }


  function showTooltip({x, y, cssClass, value, dateTime, label}) {
    const tooltip = document.createElement('div');
    tooltip.id = 'heatmap-tooltip';
    tooltip.className = cssClass;

    const timeHtml = `<div class="date">${dateTime.join('<br>')}</div>`
    const labelHtml = `<div>Name: ${label || 'value'}</div>`
    const valueHtml = `<div>Value: <strong>${value}</strong></div>`
    tooltip.innerHTML = `<div>${timeHtml}<div>${labelHtml}${valueHtml}</div></div>`;

    tooltip.style.position = 'absolute';
    tooltip.style.top = y + 5 + 'px';
    tooltip.style.left = x + 5 + 'px';
    tooltip.style.display = 'none';
    document.body.appendChild(tooltip);

    const totalTipWidth = $(tooltip).outerWidth();
    const totalTipHeight = $(tooltip).outerHeight();

    if (x > ($(window).width() - totalTipWidth)) {
      x -= totalTipWidth;
      tooltip.style.left = x + 'px';
    }

    if (y > ($(window).height() - totalTipHeight)) {
      y -= totalTipHeight;
      tooltip.style.top = y + 'px';
    }

    tooltip.style.display = 'block';  // This will trigger a re-render, allowing fadeIn to work
    tooltip.style.opacity = 1;
  }

  function removeTooltip() {
    let tooltip = document.getElementById('tooltip');
    if (tooltip) {
      document.body.removeChild(tooltip);
    }
  }

  function drawHeatmap(series, plot, ctx, seriesIndex, fills) {
    const {data: dataPoints} = series;
    const {left: xOffset, top: yOffset} = plot.getPlotOffset();
    const plotHeight = plot.height();
    const xaxis = plot.getXAxes()[0];
    const cellHeight = plotHeight / plot.getData().length;

    ctx.save();
    ctx.translate(xOffset, yOffset);

    for (let i = 0, len = dataPoints.length - 1; i < len; i++) {
      const [xStartVal, countStart] = dataPoints[i];
      const [xEndVal] = dataPoints[i + 1];

      const xStart = xaxis.p2c(xStartVal);
      const xEnd = xaxis.p2c(xEndVal);
      const cellWidth = xEnd - xStart;
      const yStart = plotHeight - (seriesIndex + 1) * cellHeight;

      ctx.fillStyle = fills[countStart];
      ctx.fillRect(xStart + 0.5, yStart + 0.5, cellWidth - 1, cellHeight - 1);
    }

    ctx.restore();
  }

  function countsToFills(counts, fillPalette) {
    const hideThreshold = 0;
    const minCount = Math.min(...counts.filter(count => count > hideThreshold));
    const maxCount = Math.max(...counts);
    const range = maxCount - minCount;
    const paletteSize = fillPalette.length;

    return counts.reduce((acc, count) => {
      const index = count === 0
        ? -1
        : Math.min(paletteSize - 1, Math.floor((paletteSize * (count - minCount)) / range));
      acc[count] = fillPalette[index] || "transparent";
      return acc;
    }, {});
  }

  function generateGradient(color1, color2, color3) {
    function interpolateColor(startColor, endColor, step) {
      let r = startColor[0] + step * (endColor[0] - startColor[0]);
      let g = startColor[1] + step * (endColor[1] - startColor[1]);
      let b = startColor[2] + step * (endColor[2] - startColor[2]);

      return `rgb(${Math.round(r)}, ${Math.round(g)}, ${Math.round(b)})`;
    }

    function hexToRgb(hex) {
      const bigint = parseInt(hex.slice(1), 16);
      const r = (bigint >> 16) & 255;
      const g = (bigint >> 8) & 255;
      const b = bigint & 255;

      return [r, g, b];
    }

    const colorList = [];
    const stepsBetween = 8; // We will get 7 intermediate colors + the starting and ending colors

    // Generating gradient between the first and second color
    for (let i = 0; i < stepsBetween; i++) {
      colorList.push(interpolateColor(hexToRgb(color1), hexToRgb(color2), i / (stepsBetween - 1)));
    }

    // Generating gradient between the second and third color
    for (let i = 1; i < stepsBetween; i++) {
      colorList.push(interpolateColor(hexToRgb(color2), hexToRgb(color3), i / (stepsBetween - 1)));
    }

    return colorList;
  }


  jQuery.plot.plugins.push({
    init,
    options: {
      series: {
        heatmap: false
      }
    },
    name: 'heatmap',
    version: '1.0'
  });
})(jQuery);
