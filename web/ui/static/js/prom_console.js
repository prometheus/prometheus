/*
 * Functions to make it easier to write prometheus consoles, such
 * as graphs.
 *
 */

PromConsole = {};

PromConsole.NumberFormatter = {};
PromConsole.NumberFormatter.prefixesBig = ["k", "M", "G", "T", "P", "E", "Z", "Y"];
PromConsole.NumberFormatter.prefixesBig1024 = ["ki", "Mi", "Gi", "Ti", "Pi", "Ei", "Zi", "Yi"];
PromConsole.NumberFormatter.prefixesSmall = ["m", "u", "n", "p", "f", "a", "z", "y"];

PromConsole._stripTrailingZero = function(x) {
    if (x.indexOf("e") == -1) {
      // It's not safe to strip if it's scientific notation.
      return x.replace(/\.?0*$/, '');
    }
    return x;
};

// Humanize a number.
PromConsole.NumberFormatter.humanize = function(x) {
  var ret = PromConsole.NumberFormatter._humanize(
    x, PromConsole.NumberFormatter.prefixesBig,
    PromConsole.NumberFormatter.prefixesSmall, 1000);
  x = ret[0];
  var prefix = ret[1];
  if (Math.abs(x) < 1) {
    return x.toExponential(3) + prefix;
  }
  return PromConsole._stripTrailingZero(x.toFixed(3)) + prefix;
};

// Humanize a number, don't use milli/micro/etc. prefixes.
PromConsole.NumberFormatter.humanizeNoSmallPrefix = function(x) {
  if (Math.abs(x) < 1) {
    return PromConsole._stripTrailingZero(x.toPrecision(3));
  }
  var ret = PromConsole.NumberFormatter._humanize(
    x, PromConsole.NumberFormatter.prefixesBig,
    [], 1000);
  x = ret[0];
  var prefix = ret[1];
  return PromConsole._stripTrailingZero(x.toFixed(3)) + prefix;
};

// Humanize a number with 1024 as the base, rather than 1000.
PromConsole.NumberFormatter.humanize1024 = function(x) {
  var ret = PromConsole.NumberFormatter._humanize(
    x, PromConsole.NumberFormatter.prefixesBig1024,
    [], 1024);
  x = ret[0];
  var prefix = ret[1];
  if (Math.abs(x) < 1) {
    return x.toExponential(3) + prefix;
  }
  return PromConsole._stripTrailingZero(x.toFixed(3)) + prefix;
};

// Humanize a number, returning an exact representation.
PromConsole.NumberFormatter.humanizeExact = function(x) {
  var ret = PromConsole.NumberFormatter._humanize(
    x, PromConsole.NumberFormatter.prefixesBig,
    PromConsole.NumberFormatter.prefixesSmall, 1000);
  return ret[0] + ret[1];
};

PromConsole.NumberFormatter._humanize = function(x, prefixesBig, prefixesSmall, factor) {
  var prefix = "";
  if (x === 0) {
    /* Do nothing. */
  } else if (Math.abs(x) >= 1) {
    for (var i=0; i < prefixesBig.length && Math.abs(x) >= factor; ++i) {
      x /= factor;
      prefix = prefixesBig[i];
    }
  } else {
    for (var i=0; i < prefixesSmall.length && Math.abs(x) < 1; ++i) {
      x *= factor;
      prefix = prefixesSmall[i];
    }
  }
  return [x, prefix];
};


PromConsole.TimeControl = function() {
  document.getElementById("prom_graph_duration_shrink").onclick = this.decreaseDuration.bind(this);
  document.getElementById("prom_graph_duration_grow").onclick = this.increaseDuration.bind(this);
  document.getElementById("prom_graph_time_back").onclick = this.decreaseEnd.bind(this);
  document.getElementById("prom_graph_time_forward").onclick = this.increaseEnd.bind(this);
  document.getElementById("prom_graph_refresh_button").onclick = this.refresh.bind(this);
  this.durationElement = document.getElementById("prom_graph_duration");
  this.endElement = document.getElementById("prom_graph_time_end");
  this.durationElement.oninput = this.dispatch.bind(this);
  this.endElement.oninput = this.dispatch.bind(this);
  this.endElement.oninput = this.dispatch.bind(this);
  this.refreshValueElement = document.getElementById("prom_graph_refresh_button_value");

  var refreshList = document.getElementById("prom_graph_refresh_intervals");
  var refreshIntervals = ["Off", "1m", "5m", "15m", "1h"];
  for (var i=0; i < refreshIntervals.length; ++i) {
    var li = document.createElement("li");
    li.onclick = this.setRefresh.bind(this, refreshIntervals[i]);
    li.textContent = refreshIntervals[i];
    refreshList.appendChild(li);
  }

  this.durationElement.value = PromConsole.TimeControl.prototype.getHumanDuration(
    PromConsole.TimeControl._initialValues.duration);
  if (!PromConsole.TimeControl._initialValues.endTimeNow) {
    this.endElement.value = PromConsole.TimeControl.prototype.getHumanDate(
      new Date(PromConsole.TimeControl._initialValues.endTime * 1000));
  }
};

PromConsole.TimeControl.timeFactors = {
  "y": 60 * 60 * 24 * 365,
  "w": 60 * 60 * 24 * 7,
  "d": 60 * 60 * 24,
  "h": 60 * 60,
  "m": 60,
  "s": 1
};

PromConsole.TimeControl.stepValues = [
  "10s", "1m", "5m", "15m", "30m", "1h", "2h", "6h", "12h", "1d", "2d",
  "1w", "2w", "4w", "8w", "1y", "2y"
];

PromConsole.TimeControl.prototype._setHash = function() {
  var duration = this.parseDuration(this.durationElement.value);
  var endTime = this.getEndDate() / 1000;
  window.location.hash = "#pctc" + encodeURIComponent(JSON.stringify(
    {duration: duration, endTime: endTime}));
};

PromConsole.TimeControl._initialValues = function() {
  var hash = window.location.hash;
  var values;
  if (hash.indexOf('#pctc') === 0) {
    values = JSON.parse(decodeURIComponent(hash.substring(5)));
  } else {
    values = {duration: 3600, endTime: 0};
  }
  if (values.endTime == 0) {
    values.endTime = new Date().getTime() / 1000;
    values.endTimeNow = true;
  }
  return values;
}();

PromConsole.TimeControl.prototype.parseDuration = function(durationText) {
  var durationRE = new RegExp("^([0-9]+)([ywdhms]?)$");
  var matches = durationText.match(durationRE);
  if (!matches) { return 3600; }
  var value = parseInt(matches[1]);
  var unit = matches[2] || 's';
  return value * PromConsole.TimeControl.timeFactors[unit];
};

PromConsole.TimeControl.prototype.getHumanDuration = function(duration) {
  var units = [];
  for (var key in PromConsole.TimeControl.timeFactors) {
    units.push([PromConsole.TimeControl.timeFactors[key], key]);
  }
  units.sort(function(a, b) { return b[0] - a[0]; });
  for (var i = 0; i < units.length; ++i) {
    if (duration % units[i][0] === 0) {
      return (duration / units[i][0]) + units[i][1];
    }
  }
  return duration;
};

PromConsole.TimeControl.prototype.increaseDuration = function() {
  var durationSeconds = this.parseDuration(this.durationElement.value);
  for (var i = 0; i < PromConsole.TimeControl.stepValues.length; i++) {
    if (durationSeconds < this.parseDuration(PromConsole.TimeControl.stepValues[i])) {
      this.setDuration(PromConsole.TimeControl.stepValues[i]);
      this.dispatch();
      return;
    }
  }
};

PromConsole.TimeControl.prototype.decreaseDuration = function() {
  var durationSeconds = this.parseDuration(this.durationElement.value);
  for (var i = PromConsole.TimeControl.stepValues.length - 1; i >= 0; i--) {
    if (durationSeconds > this.parseDuration(PromConsole.TimeControl.stepValues[i])) {
      this.setDuration(PromConsole.TimeControl.stepValues[i]);
      this.dispatch();
      return;
    }
  }
};

PromConsole.TimeControl.prototype.setDuration = function(duration) {
  this.durationElement.value = duration;
  this._setHash();
};

PromConsole.TimeControl.prototype.getEndDate = function() {
  if (this.endElement.value === '') {
    return null;
  }
  var dateParts = this.endElement.value.split(/[- :]/)
  return Date.UTC(dateParts[0], dateParts[1] - 1, dateParts[2], dateParts[3], dateParts[4]);
};

PromConsole.TimeControl.prototype.getOrSetEndDate = function() {
  var date = this.getEndDate();
  if (date) {
    return date;
  }
  date = new Date();
  this.setEndDate(date);
  return date.getTime();
};

PromConsole.TimeControl.prototype.getHumanDate = function(date) {
  var hours = date.getUTCHours() < 10 ? '0' + date.getUTCHours() : date.getUTCHours();
  var minutes = date.getUTCMinutes() < 10 ? '0' + date.getUTCMinutes() : date.getUTCMinutes();
  return date.getUTCFullYear() + "-" + (date.getUTCMonth()+1) + "-" + date.getUTCDate() + " " +
      hours + ":" + minutes;
};

PromConsole.TimeControl.prototype.setEndDate = function(date) {
  this.setRefresh("Off");
  this.endElement.value = this.getHumanDate(date);
  this._setHash();
};

PromConsole.TimeControl.prototype.increaseEnd = function() {
  // Increase duration 25% range & convert ms to s.
  this.setEndDate(new Date(this.getOrSetEndDate() + this.parseDuration(this.durationElement.value) * 1000/4 ));
  this.dispatch();
};

PromConsole.TimeControl.prototype.decreaseEnd = function() {
  this.setEndDate(new Date(this.getOrSetEndDate() - this.parseDuration(this.durationElement.value) * 1000/4 ));
  this.dispatch();
};

PromConsole.TimeControl.prototype.refresh = function() {
  this.endElement.value = '';
  this._setHash();
  this.dispatch();
};

PromConsole.TimeControl.prototype.dispatch = function() {
  var durationSeconds = this.parseDuration(this.durationElement.value);
  var end = this.getEndDate();
  if (end === null) {
    end = new Date().getTime();
  }
  for (var i = 0; i< PromConsole._graph_registry.length; i++) {
    var graph = PromConsole._graph_registry[i];
    graph.params.duration = durationSeconds;
    graph.params.endTime = end / 1000;
    graph.dispatch();
  }
};

PromConsole.TimeControl.prototype._refreshInterval = null;

PromConsole.TimeControl.prototype.setRefresh = function(duration) {
  if (this._refreshInterval !== null) {
    window.clearInterval(this._refreshInterval);
    this._refreshInterval = null;
  }
  if (duration != "Off") {
    if (this.endElement.value !== '') {
      this.refresh();
    }
    var durationSeconds = this.parseDuration(duration);
    this._refreshInterval = window.setInterval(this.dispatch.bind(this), durationSeconds * 1000);
  }
  this.refreshValueElement.textContent = duration;
};

// List of all graphs, used by time controls.
PromConsole._graph_registry = [];

PromConsole.graphDefaults = {
  expr: null,     // Expression to graph. Can be a list of strings.
  node: null,     // DOM node to place graph under.
                  // How long the graph is over, in seconds.
  duration: PromConsole.TimeControl._initialValues.duration,
                  // The unixtime the graph ends at.
  endTime:  PromConsole.TimeControl._initialValues.endTime,
  width: null,    // Height of the graph div, excluding titles and legends.
                  // Defaults to auto-detection.
  height: 200,    // Height of the graph div, excluding titles and legends.
  min: "auto",    // Minimum Y-axis value, defaults to lowest data value.
  max: undefined, // Maximum Y-axis value, defaults to highest data value.
  renderer: 'line',  // Type of graphs, options are 'line' and 'area'.
  name: null,     // What to call plots, defaults to trying to do
                  // something reasonable.
                  // If a string, it'll use that. [[ label ]] will be substituted.
                  // If a function it'll be called with a map of keys to values,
                  // and should return the name to use.
                  // Can be a list of strings/functions, each element
                  // will be applied to the plots from the corresponding
                  // element of the expr list.
  xTitle: "Time",     // The title of the x axis.
  yUnits: "",     // The units of the y axis.
  yTitle: "",     // The title of the y axis.
  // Number formatter for y axis.
  yAxisFormatter: PromConsole.NumberFormatter.humanize,
  // Number formatter for y values hover detail.
  yHoverFormatter: PromConsole.NumberFormatter.humanizeExact,
};

PromConsole.Graph = function(params) {
  for (var k in PromConsole.graphDefaults) {
    if (!(k in params)) {
      params[k] = PromConsole.graphDefaults[k];
    }
  }
  if (typeof params.expr == "string") {
    params.expr = [params.expr];
  }
  if (typeof params.name == "string" || typeof params.name == "function") {
    var name = [];
    for (var i = 0; i < params.expr.length; i++) {
      name.push(params.name);
    }
    params.name = name;
  }

  this.params = params;
  this.rendered_data = null;
  // Keep a reference so that further updates (e.g. annotations) can be made
  // by the user in their templates.
  this.rickshawGraph = null;
  PromConsole._graph_registry.push(this);

  /*
   * Table layout:
   * | yTitle | Graph  |
   * |        | xTitle |
   * | /graph | Legend |
   */
  var table = document.createElement("table");
  table.className = "prom_graph_table";
  params.node.appendChild(table);
  var tr = document.createElement("tr");
  table.appendChild(tr);
  var yTitleTd = document.createElement("td");
  tr.appendChild(yTitleTd);
  var yTitleDiv = document.createElement("td");
  yTitleTd.appendChild(yTitleDiv);
  yTitleDiv.className = "prom_graph_ytitle";
  yTitleDiv.textContent = params.yTitle + (params.yUnits ? " (" + params.yUnits.trim() + ")" : "");

  this.graphTd = document.createElement("td");
  tr.appendChild(this.graphTd);
  this.graphTd.className = "rickshaw_graph";
  this.graphTd.width = params.width;
  this.graphTd.height = params.height;

  tr = document.createElement("tr");
  table.appendChild(tr);
  tr.appendChild(document.createElement("td"));
  var xTitleTd = document.createElement("td");
  tr.appendChild(xTitleTd);
  xTitleTd.className = "prom_graph_xtitle";
  xTitleTd.textContent = params.xTitle;

  tr = document.createElement("tr");
  table.appendChild(tr);
  var graphLinkTd = document.createElement("td");
  tr.appendChild(graphLinkTd);
  var graphLinkA = document.createElement("a");
  graphLinkTd.appendChild(graphLinkA);
  graphLinkA.className = "prom_graph_link";
  graphLinkA.textContent = "+";
  graphLinkA.href = PromConsole._graphsToSlashGraphURL(params.expr);
  var legendTd = document.createElement("td");
  tr.appendChild(legendTd);
  this.legendDiv = document.createElement("div");
  legendTd.width = params.width;
  legendTd.appendChild(this.legendDiv);

  window.addEventListener('resize', function() {
    if(this.rendered_data !== null) {
      this._render(this.rendered_data);
    }
  }.bind(this));

  this.dispatch();
};

PromConsole.Graph.prototype._parseValue = function(value) {
  var val = parseFloat(value);
  if (isNaN(val)) {
    // "+Inf", "-Inf", "+Inf" will be parsed into NaN by parseFloat(). The
    // can't be graphed, so show them as gaps (null).
    return null;
  }
  return val;
};

PromConsole.Graph.prototype._escapeHTML = function(string) {
  var entityMap = {
    "&": "&amp;",
    "<": "&lt;",
    ">": "&gt;",
    '"': '&quot;',
    "'": '&#39;',
    "/": '&#x2F;'
  };

  return string.replace(/[&<>"'\/]/g, function (s) {
    return entityMap[s];
  });
};

PromConsole.Graph.prototype._render = function(data) {
  var self = this;
  var palette = new Rickshaw.Color.Palette();
  var series = [];

  // This will be used on resize.
  this.rendered_data = data;


  var nameFuncs = [];
  if (this.params.name === null) {
    var chooser = PromConsole._chooseNameFunction(data);
    for (var i = 0; i < this.params.expr.length; i++) {
      nameFuncs.push(chooser);
    }
  } else {
    for (var i = 0; i < this.params.name.length; i++) {
      if (typeof this.params.name[i] == "string") {
        nameFuncs.push(function(i, metric) {
          return PromConsole._interpolateName(this.params.name[i], metric);
        }.bind(this, i));
      } else {
        nameFuncs.push(this.params.name[i]);
      }
    }
  }

  // Get the data into the right format.
  var seriesLen = 0;
  for (var e = 0; e < data.length; e++) {
    for (var i = 0; i < data[e].data.result.length; i++) {
      series[seriesLen++] = {
            data: data[e].data.result[i].values.map(function(s) { return {x: s[0], y: self._parseValue(s[1])}; }),
            color: palette.color(),
            name: self._escapeHTML(nameFuncs[e](data[e].data.result[i].metric)),
      };
    }
  }
  this._clearGraph();
  if (!series.length) {
    var errorText = document.createElement("div");
    errorText.className = 'prom_graph_error';
    errorText.textContent = 'No timeseries returned';
    this.graphTd.appendChild(errorText);
    return;
  }
  // Render.
  var graph = new Rickshaw.Graph({
          interpolation: "linear",
          width: this.graphTd.offsetWidth,
          height: this.params.height,
          element: this.graphTd,
          renderer: this.params.renderer,
          max: this.params.max,
          min: this.params.min,
          series: series
  });
  var hoverDetail = new Rickshaw.Graph.HoverDetail({
      graph: graph,
      onRender: function() {
        var xLabel = this.element.getElementsByClassName("x_label")[0];
        var item = this.element.getElementsByClassName("item")[0];
        if (xLabel.offsetWidth + xLabel.offsetLeft + this.element.offsetLeft > graph.element.offsetWidth ||
          item.offsetWidth + item.offsetLeft + this.element.offsetLeft > graph.element.offsetWidth) {
          xLabel.classList.add("prom_graph_hover_flipped");
          item.classList.add("prom_graph_hover_flipped");
        } else {
          xLabel.classList.remove("prom_graph_hover_flipped");
          item.classList.remove("prom_graph_hover_flipped");
        }
      },
      yFormatter: function(y) {
        return this.params.yHoverFormatter(y) + this.params.yUnits;
      }.bind(this)
  });
  var yAxis = new Rickshaw.Graph.Axis.Y({
      graph: graph,
      tickFormat: this.params.yAxisFormatter
  });
  var xAxis = new Rickshaw.Graph.Axis.Time({
      graph: graph,
  });
  var legend = new Rickshaw.Graph.Legend({
      graph: graph,
      element: this.legendDiv
  });
  xAxis.render();
  yAxis.render();
  graph.render();

  this.rickshawGraph = graph;
};

PromConsole.Graph.prototype._clearGraph = function() {
  while (this.graphTd.lastChild) {
    this.graphTd.removeChild(this.graphTd.lastChild);
  }
  while (this.legendDiv.lastChild) {
    this.legendDiv.removeChild(this.legendDiv.lastChild);
  }
  this.rickshawGraph = null;
};

PromConsole.Graph.prototype._xhrs = [];

PromConsole.Graph.prototype.buildQueryUrl = function(expr) {
  var p = this.params;
  return PATH_PREFIX + "/api/v1/query_range?query=" +
    encodeURIComponent(expr) +
    "&step=" + p.duration / this.graphTd.offsetWidth +
    "&start=" + (p.endTime - p.duration) + "&end=" + p.endTime;
};

PromConsole.Graph.prototype.dispatch = function() {
  for (var j = 0; j < this._xhrs.length; j++) {
    this._xhrs[j].abort();
  }
  var all_data = new Array(this.params.expr.length);
  this._xhrs = new Array(this.params.expr.length);
  var pending_requests = this.params.expr.length;
  for (var i = 0; i < this.params.expr.length; ++i) {
    var endTime = this.params.endTime;
    var url = this.buildQueryUrl(this.params.expr[i]);
    var xhr = new XMLHttpRequest();
    xhr.open('get', url, true);
    xhr.responseType = 'json';
    xhr.onerror = function(xhr, i) {
      this._clearGraph();
      var errorText = document.createElement("div");
      errorText.className = 'prom_graph_error';
      errorText.textContent = 'Error loading data';
      this.graphTd.appendChild(errorText);
      console.log('Error loading data for ' + this.params.expr[i]);
      pending_requests = 0;
      // onabort gets any aborts.
      for (var j = 0; j < pending_requests; j++) {
        this._xhrs[j].abort();
      }
    }.bind(this, xhr, i);
    xhr.onload = function(xhr, i) {
      if (pending_requests === 0) {
        // Got an error before this success.
        return;
      }
      var data = xhr.response;
      if (typeof data !== "object") {
        data = JSON.parse(xhr.responseText);
      }
      pending_requests -= 1;
      all_data[i] = data;
      if (pending_requests === 0) {
        this._xhrs = [];
        this._render(all_data);
      }
    }.bind(this, xhr, i);
    xhr.send();
    this._xhrs[i] = xhr;
  }

  var loadingImg = document.createElement("img");
  loadingImg.src = PATH_PREFIX + '/static/img/ajax-loader.gif';
  loadingImg.alt = 'Loading...';
  loadingImg.className = 'prom_graph_loading';
  this.graphTd.appendChild(loadingImg);
};

// Substitute the value of 'label' for [[ label ]].
PromConsole._interpolateName = function(name, metric) {
  var re = /(.*?)\[\[\s*(\w+)+\s*\]\](.*?)/g;
  var result = '';
  while (match = re.exec(name)) {
    result = result + match[1] + metric[match[2]] + match[3];
  }
  if (!result) {
    return name;
  }
  return result;
};

// Given the data returned by the API, return an appropriate function
// to return plot names.
PromConsole._chooseNameFunction = function(data) {
  // By default, use the full metric name.
  var nameFunc = function (metric) {
    name = metric.__name__ + "{";
    for (var label in metric) {
      if (label.substring(0,2) == "__") {
        continue;
      }
      name += label + "='" + metric[label] + "',";
    }
    return name + "}";
  };
  
  // If only one label varies, use that value.
  var labelValues = {};
  for (var e = 0; e < data.length; e++) {
    for (var i = 0; i < data[e].data.result.length; i++) {
      for (var label in data[e].data.result[i].metric) {
        if (!(label in labelValues)) {
          labelValues[label] = {};
        }
        labelValues[label][data[e].data.result[i].metric[label]] = 1;
      }
    }
  }
  
  var multiValueLabels = [];
  for (var label in labelValues) {
    if (Object.keys(labelValues[label]).length > 1) {
      multiValueLabels.push(label);
    }
  }
  if (multiValueLabels.length == 1) {
    nameFunc = function(metric) {
      return metric[multiValueLabels[0]];
    };
  }
  return nameFunc;
};

// Given a list of expressions, produce the /graph url for them.
PromConsole._graphsToSlashGraphURL = function(exprs) {
  var data = [];
  for (var i = 0; i < exprs.length; ++i) {
    data.push({'expr': exprs[i], 'tab': 0});
  }
  return PATH_PREFIX + '/graph#' + encodeURIComponent(JSON.stringify(data));

};
