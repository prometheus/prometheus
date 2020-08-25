var Prometheus = Prometheus || {};
var graphTemplate;

var INPUT_DEBOUNCE_WAIT = 500; // ms
var SECOND = 1000;

/**
 * Graph
*/
Prometheus.Graph = function(element, options, handleChange, handleRemove) {
  this.el = element;
  this.graphHTML = null;
  this.options = options;
  this.handleChange = handleChange;
  this.handleRemove = function() {
    handleRemove(this);
  };
  this.rickshawGraph = null;
  this.data = [];

  this.initialize();
};

Prometheus.Graph.timeFactors = {
  "y": 60 * 60 * 24 * 365,
  "w": 60 * 60 * 24 * 7,
  "d": 60 * 60 * 24,
  "h": 60 * 60,
  "m": 60,
  "s": 1
};

Prometheus.Graph.stepValues = [
  "1s", "10s", "1m", "5m", "15m", "30m", "1h", "2h", "6h", "12h", "1d", "2d",
  "1w", "2w", "4w", "8w", "1y", "2y"
];

Prometheus.Graph.numGraphs = 0;

Prometheus.Graph.prototype.initialize = function() {
  var self = this;
  self.id = Prometheus.Graph.numGraphs++;

  // Set default options.
  self.options.id = self.id;
  self.options.range_input = self.options.range_input || "1h";
  if (self.options.tab === undefined) {
    self.options.tab = 1;
  }

  // Draw graph controls and container from Handlebars template.

  var options = {
    'pathPrefix': PATH_PREFIX,
    'buildVersion': BUILD_VERSION
  };
  jQuery.extend(options, self.options);
  self.graphHTML = $(Mustache.render(graphTemplate, options));
  self.el.append(self.graphHTML);

  // Get references to all the interesting elements in the graph container and
  // bind event handlers.
  var graphWrapper = self.el.find("#graph_wrapper" + self.id);
  self.queryForm = graphWrapper.find(".query_form");

  // Auto-resize the text area on input or mouseclick
  var resizeTextarea = function(el) {
    var offset = el.offsetHeight - el.clientHeight;
    $(el).css('height', 'auto').css('height', el.scrollHeight + offset);
  };

  self.expr = graphWrapper.find("textarea[name=expr]");
  self.expr.keypress(function(e) {
    const enter = 13;
    if (e.which == enter && !e.shiftKey) {
      self.queryForm.submit();
      e.preventDefault();
    }
    $(this).on('keyup input', debounce(function() {
      resizeTextarea(this);
    }, INPUT_DEBOUNCE_WAIT));
  });

  self.expr.click(function(e) {
    resizeTextarea(this);
  });

  self.expr.change(self.handleChange);

  self.rangeInput = self.queryForm.find("input[name=range_input]");
  self.stackedBtn = self.queryForm.find(".stacked_btn");
  self.stacked = self.queryForm.find("input[name=stacked]");
  self.insertMetric = self.queryForm.find("select[name=insert_metric]");
  self.refreshInterval = self.queryForm.find("select[name=refresh]");

  self.consoleTab = graphWrapper.find(".console");
  self.graphTab   = graphWrapper.find(".graph_container");

  self.tabs = graphWrapper.find("a[data-toggle='tab']");
  self.tabs.eq(self.options.tab).tab("show");
  self.tabs.on("shown.bs.tab", function(e) {
    var target = $(e.target);
    self.options.tab = target.parent().index();
    self.handleChange();
    if ($("#" + target.attr("aria-controls")).hasClass("reload")) {
      self.submitQuery();
    }
  });

  // Return moves focus back to expr instead of submitting.
  self.insertMetric.bind("keydown", "return", function(e) {
    self.expr.focus();
    self.expr.val(self.expr.val());
    return e.preventDefault();
  })

  self.error = graphWrapper.find(".error").hide();
  self.warning = graphWrapper.find(".warning").hide();
  self.graphArea = graphWrapper.find(".graph_area");
  self.graph = self.graphArea.find(".graph");
  self.yAxis = self.graphArea.find(".y_axis");
  self.legend = graphWrapper.find(".legend");
  self.spinner = graphWrapper.find(".spinner");
  self.evalStats = graphWrapper.find(".eval_stats");

  self.endDate = graphWrapper.find("input[name=end_input]");
  self.endDate.datetimepicker({
    locale: 'en',
    format: 'YYYY-MM-DD HH:mm',
    toolbarPlacement: 'bottom',
    sideBySide: true,
    showTodayButton: true,
    showClear: true,
    showClose: true,
    timeZone: 'UTC',
  });
  if (self.options.end_input) {
    self.endDate.data('DateTimePicker').date(self.options.end_input);
  }
  self.endDate.on("dp.change", function() { self.submitQuery(); });
  self.refreshInterval.change(function() { self.updateRefresh(); });

  self.isStacked = function() {
    return self.stacked.val() === '1';
  };

  self.moment = graphWrapper.find("input[name=moment_input]");
  self.moment.datetimepicker({
    locale: 'en',
    format: 'YYYY-MM-DD HH:mm:ss',
    toolbarPlacement: 'bottom',
    sideBySide: true,
    showTodayButton: true,
    showClear: true,
    showClose: true,
    timeZone: 'UTC',
  });
  if (self.options.timestamp) {
    var date = new Date(self.options.timestamp*1000)
    self.moment.data('DateTimePicker').date(date);
  } else if (self.options.moment_input) {
    self.moment.data('DateTimePicker').date(self.options.moment_input);
  }
  self.moment.on("dp.change", function() { self.submitQuery(); });


  var styleStackBtn = function() {
    var icon = self.stackedBtn.find('.glyphicon');
    if (self.isStacked()) {
      self.stackedBtn.addClass("btn-primary");
      icon.addClass("glyphicon-check");
      icon.removeClass("glyphicon-unchecked");
    } else {
      self.stackedBtn.removeClass("btn-primary");
      icon.addClass("glyphicon-unchecked");
      icon.removeClass("glyphicon-check");
    }
  };
  styleStackBtn();

  self.stackedBtn.click(function() {
    if (self.isStacked() && self.graphJSON) {
      // If the graph was stacked, the original series data got mutated
      // (scaled) and we need to reconstruct it from the original JSON data.
      self.data = self.transformData(self.graphJSON);
    }
    self.stacked.val(self.isStacked() ? '0' : '1');
    styleStackBtn();
    self.updateGraph();
  });

  self.queryForm.submit(function() {
    self.consoleTab.addClass("reload");
    self.graphTab.addClass("reload");
    self.handleChange();
    self.submitQuery();
    return false;
  });
  self.spinner.hide();

  self.queryForm.find("button[name=inc_range]").click(function() { self.increaseRange(); });
  self.queryForm.find("button[name=dec_range]").click(function() { self.decreaseRange(); });

  self.queryForm.find("button[name=inc_end]").click(function() { self.increaseEnd(); });
  self.queryForm.find("button[name=dec_end]").click(function() { self.decreaseEnd(); });

  self.queryForm.find("button[name=inc_moment]").click(function() { self.increaseMoment(); });
  self.queryForm.find("button[name=dec_moment]").click(function() { self.decreaseMoment(); });

  self.insertMetric.change(function() {
    self.expr.selection("replace", {text: self.insertMetric.val(), mode: "before"});
    self.expr.focus(); // refocusing
  });

  var removeBtn = graphWrapper.find("[name=remove]");
  removeBtn.click(function() {
    self.remove();
    return false;
  });

  self.checkTimeDrift();
  // initialize query history
  if (!localStorage.getItem("history")) {
    localStorage.setItem("history", JSON.stringify([]));
  }
  self.populateInsertableMetrics();

  if (self.expr.val()) {
    self.submitQuery();
  }
};

Prometheus.Graph.prototype.checkTimeDrift = function() {
    var self = this;
    var browserTime = new Date().getTime() / 1000;
    $.ajax({
        method: "GET",
        url: PATH_PREFIX + "/api/v1/query?query=time()",
        dataType: "json",
        success: function(json, textStatus) {
            if (json.status !== "success") {
                self.showError("Error querying time.");
                return;
            }
            var serverTime = json.data.result[0];
            var diff = Math.abs(browserTime - serverTime);

            if (diff >= 30) {
              self.showWarning(
                  "<div class=\"alert alert-warning\"><strong>Warning!</strong> Detected " +
                  diff.toFixed(2) +
                  " seconds time difference between your browser and the server. Prometheus relies on accurate time and time drift might cause unexpected query results.</div>"
              );
            }
        },
        error: function() {
            self.showError("Error loading time.");
        }
    });
};

Prometheus.Graph.prototype.populateInsertableMetrics = function() {
  var self = this;
  $.ajax({
      method: "GET",
      url: PATH_PREFIX + "/api/v1/label/__name__/values",
      dataType: "json",
      success: function(json, textStatus) {
        if (json.status !== "success") {
          self.showError("Error loading available metrics!");
          return;
        }

        pageConfig.allMetrics = json.data || []; // todo: do we need self.allMetrics? Or can it just live on the page
        for (var i = 0; i < pageConfig.allMetrics.length; i++) {
          self.insertMetric[0].options.add(new Option(pageConfig.allMetrics[i], pageConfig.allMetrics[i]));
        }

        self.fuzzyResult = {
          query: null,
          result: null,
          map: {}
        }

        self.initTypeahead(self);
      },
      error: function() {
        self.showError("Error loading available metrics!");
      },
  });
};

Prometheus.Graph.prototype.initTypeahead = function(self) {
  const source = queryHistory.isEnabled() ? pageConfig.queryHistMetrics.concat(pageConfig.allMetrics) : pageConfig.allMetrics;
  self.expr.typeahead({
    autoSelect: false,
    source: source,
    items: 1000,
    matcher: function (item) {
      // If we have result for current query, skip
      if (self.fuzzyResult.query !== this.query) {
        self.fuzzyResult.query = this.query;
        self.fuzzyResult.map = {};
        self.fuzzyResult.result = fuzzy.filter(this.query.replace(/ /g, ""), this.source, {
          pre: "<strong>",
          post: "</strong>"
        });
        self.fuzzyResult.result.forEach(function(r) {
          self.fuzzyResult.map[r.original] = r;
        });
      }

      return item in self.fuzzyResult.map;
    },

    sorter: function (items) {
      items.sort(function(a,b) {
        var i = self.fuzzyResult.map[b].score - self.fuzzyResult.map[a].score;
        return i === 0 ? a.localeCompare(b) : i;
      });
      return items;
    }
  });
  // This needs to happen after attaching the typeahead plugin, as it
  // otherwise breaks the typeahead functionality.
  self.expr.focus();
  queryHistory.bindHistoryEvents(self);
}

Prometheus.Graph.prototype.getOptions = function() {
  var self = this;
  var options = {};

  var optionInputs = [
    "range_input",
    "end_input",
    "step_input",
    "stacked",
    "moment_input"
  ];

  self.queryForm.find("input").each(function(index, element) {
    var name = element.name;
    if ($.inArray(name, optionInputs) >= 0) {
      if (element.value.length > 0) {
        options[name] = element.value;
      }
    }
  });
  options.expr = self.expr.val();
  options.tab = self.options.tab;
  return options;
};

Prometheus.Graph.prototype.parseDuration = function(rangeText) {
  var rangeRE = new RegExp("^([0-9]+)([ywdhms]+)$");
  var matches = rangeText.match(rangeRE);
  if (!matches) { return; }
  if (matches.length != 3) {
    return 60;
  }
  var value = parseInt(matches[1]);
  var unit = matches[2];
  return value * Prometheus.Graph.timeFactors[unit];
};

Prometheus.Graph.prototype.increaseRange = function() {
  var self = this;
  var rangeSeconds = self.parseDuration(self.rangeInput.val());
  for (var i = 0; i < Prometheus.Graph.stepValues.length; i++) {
    if (rangeSeconds < self.parseDuration(Prometheus.Graph.stepValues[i])) {
      self.rangeInput.val(Prometheus.Graph.stepValues[i]);
      if (self.expr.val()) {
        self.submitQuery();
      }
      return;
    }
  }
};

Prometheus.Graph.prototype.decreaseRange = function() {
  var self = this;
  var rangeSeconds = self.parseDuration(self.rangeInput.val());
  for (var i = Prometheus.Graph.stepValues.length - 1; i >= 0; i--) {
    if (rangeSeconds > self.parseDuration(Prometheus.Graph.stepValues[i])) {
      self.rangeInput.val(Prometheus.Graph.stepValues[i]);
      if (self.expr.val()) {
        self.submitQuery();
      }
      return;
    }
  }
};

Prometheus.Graph.prototype.getEndDate = function() {
  var self = this;
  if (!self.endDate || !self.endDate.val()) {
    return moment();
  }
  return self.endDate.data('DateTimePicker').date();
};

Prometheus.Graph.prototype.getOrSetEndDate = function() {
  var self = this;
  var date = self.getEndDate();
  self.setEndDate(date);
  return date;
};

Prometheus.Graph.prototype.setEndDate = function(date) {
  var self = this;
  self.endDate.data('DateTimePicker').date(date);
};

Prometheus.Graph.prototype.increaseEnd = function() {
  var self = this;
  var newDate = moment(self.getOrSetEndDate());
  newDate.add(self.parseDuration(self.rangeInput.val()) / 2, 'seconds');
  self.setEndDate(newDate);
  self.submitQuery();
};

Prometheus.Graph.prototype.decreaseEnd = function() {
  var self = this;
  var newDate = moment(self.getOrSetEndDate());
  newDate.subtract(self.parseDuration(self.rangeInput.val()) / 2, 'seconds');
  self.setEndDate(newDate);
  self.submitQuery();
};

Prometheus.Graph.prototype.getMoment = function() {
  var self = this;
  if (!self.moment || !self.moment.val()) {
    return moment();
  }
  return self.moment.data('DateTimePicker').date();
};

Prometheus.Graph.prototype.getOrSetMoment = function() {
  var self = this;
  var date = self.getMoment();
  self.setMoment(date);
  return date;
};

Prometheus.Graph.prototype.setMoment = function(date) {
  var self = this;
  self.moment.data('DateTimePicker').date(date);
};

Prometheus.Graph.prototype.increaseMoment = function() {
  var self = this;
  var newDate = moment(self.getOrSetMoment());
  newDate.add(10, 'seconds');
  self.setMoment(newDate);
  self.submitQuery();
};

Prometheus.Graph.prototype.decreaseMoment = function() {
  var self = this;
  var newDate = moment(self.getOrSetMoment());
  newDate.subtract(10, 'seconds');
  self.setMoment(newDate);
  self.submitQuery();
};

Prometheus.Graph.prototype.submitQuery = function() {
  var self = this;
  self.clearError();
  self.clearWarning();
  if (!self.expr.val()) {
    return;
  }

  self.spinner.show();
  self.evalStats.empty();

  var startTime = new Date().getTime();
  var rangeSeconds = self.parseDuration(self.rangeInput.val());
  var resolution = parseInt(self.queryForm.find("input[name=step_input]").val()) || Math.max(Math.floor(rangeSeconds / 250), 1);
  var endDate = self.getEndDate() / 1000;
  var moment = self.getMoment() / 1000;

  if (self.queryXhr) {
    self.queryXhr.abort();
  }
  var url;
  var success;
  var params = {
    "query": self.expr.val()
  };
  if (self.options.tab === 0) {
    params.start = endDate - rangeSeconds;
    params.end = endDate;
    params.step = resolution;
    url = PATH_PREFIX + "/api/v1/query_range";
    success = function(json, textStatus) { self.handleGraphResponse(json, textStatus); };
  } else {
    params.time = moment;
    url = PATH_PREFIX + "/api/v1/query";
    success = function(json, textStatus) { self.handleConsoleResponse(json, textStatus); };
  }
  self.params = params;

  self.queryXhr = $.ajax({
      method: self.queryForm.attr("method"),
      url: url,
      dataType: "json",
      data: params,
      success: function(json, textStatus) {
        if (json.status !== "success") {
          self.showError(json.error);
          return;
        }

        if ("warnings" in json && json.warnings.length > 0) {
          self.showWarning(json.warnings.map(escapeHTML).join('<br>'));
        }

        queryHistory.handleHistory(self);
        success(json.data, textStatus);
      },
      error: function(xhr, resp) {
        if (resp != "abort") {
          var err;
          if (xhr.responseJSON !== undefined) {
            err = xhr.responseJSON.error;
          } else {
            err = xhr.statusText;
          }
          self.showError("Error executing query: " + err);
        }
      },
      complete: function(xhr, resp) {
        if (resp == "abort") {
          return;
        }
        var duration = new Date().getTime() - startTime;
        var totalTimeSeries = 0;
        if (xhr.responseJSON.data !== undefined) {
          if (xhr.responseJSON.data.resultType === "scalar") {
            totalTimeSeries = 1;
          } else if(xhr.responseJSON.data.result !== null) {
            totalTimeSeries = xhr.responseJSON.data.result.length;
          }
        }
        self.evalStats.html("Load time: " + duration + "ms <br /> Resolution: " + resolution + "s <br />" + "Total time series: " + totalTimeSeries);
        self.spinner.hide();
      }
  });
};

Prometheus.Graph.prototype.showError = function(msg) {
  var self = this;
  self.error.text(msg);
  self.error.show();
};

Prometheus.Graph.prototype.clearError = function() {
  var self = this;
  self.error.text('');
  self.error.hide();
};

Prometheus.Graph.prototype.showWarning = function(msg) {
  var self = this;
  self.warning.html(msg);
  self.warning.show();
};

Prometheus.Graph.prototype.clearWarning = function() {
  var self = this;
  self.warning.html('');
  self.warning.hide();
};

Prometheus.Graph.prototype.updateRefresh = function() {
  var self = this;

  if (self.timeoutID) {
    window.clearTimeout(self.timeoutID);
  }

  interval = self.parseDuration(self.refreshInterval.val());
  if (!interval) { return; }

  self.timeoutID = window.setTimeout(function() {
    self.submitQuery();
    self.updateRefresh();
  }, interval * SECOND);
};

Prometheus.Graph.prototype.renderLabels = function(labels) {
  var labelStrings = [];
  for (var label in labels) {
    if (label != "__name__") {
      labelStrings.push("<strong>" + label + "</strong>: " + escapeHTML(labels[label]));
    }
  }
  return labels = "<div class=\"labels\">" + labelStrings.join("<br>") + "</div>";
};

Prometheus.Graph.prototype.metricToTsName = function(labels) {
  var tsName = (labels.__name__ || '') + "{";
  var labelStrings = [];
   for (var label in labels) {
     if (label != "__name__") {
       labelStrings.push(label + "=\"" + labels[label] + "\"");
     }
   }
  tsName += labelStrings.join(",") + "}";
  return tsName;
};

Prometheus.Graph.prototype.parseValue = function(value) {
  var val = parseFloat(value);
  if (isNaN(val)) {
    // "+Inf", "-Inf", "+Inf" will be parsed into NaN by parseFloat(). The
    // can't be graphed, so show them as gaps (null).
    return null;
  }
  return val;
};

Prometheus.Graph.prototype.transformData = function(json) {
  var self = this;
  var palette = new Rickshaw.Color.Palette();
  if (json.resultType != "matrix") {
    self.showError("Result is not of matrix type! Please enter a correct expression.");
    return [];
  }
  var data = json.result.map(function(ts) {
    var name;
    var labels;
    if (ts.metric === null) {
      name = "scalar";
      labels = {};
    } else {
      name = escapeHTML(self.metricToTsName(ts.metric));
      labels = ts.metric;
    }
    return {
      name: name,
      labels: labels,
      data: ts.values.map(function(value) {
        return {
          x: value[0],
          y: self.parseValue(value[1])
        };
      }),
      color: palette.color()
    };
  });
  data.forEach(function(s) {
    // Insert nulls for all missing steps.
    var newSeries = [];
    var pos = 0;
    for (var t = self.params.start; t <= self.params.end; t += self.params.step) {
      // Allow for floating point inaccuracy.
      if (s.data.length > pos && s.data[pos].x < t + self.params.step / 100) {
        newSeries.push(s.data[pos]);
        pos++;
      } else {
        newSeries.push({x: t, y: null});
      }
    }
    s.data = newSeries;
  });
  return data;
};

Prometheus.Graph.prototype.updateGraph = function() {
  var self = this;
  if (self.data.length === 0) { return; }

  // Remove any traces of an existing graph.
  self.legend.empty();
  if (self.graphArea.children().length > 0) {
    self.graph.remove();
    self.yAxis.remove();
  }
  self.graph = $('<div class="graph"></div>');
  self.yAxis = $('<div class="y_axis"></div>');
  self.graphArea.append(self.graph);
  self.graphArea.append(self.yAxis);

  var endTime = self.getEndDate() / 1000; // Convert to UNIX timestamp.
  var duration = self.parseDuration(self.rangeInput.val()) || 3600; // 1h default.
  var startTime = endTime - duration;
  self.data.forEach(function(s) {
    // Padding series with invisible "null" values at the configured x-axis boundaries ensures
    // that graphs are displayed with a fixed x-axis range instead of snapping to the available
    // time range in the data.
    if (s.data[0].x > startTime) {
      s.data.unshift({x: startTime, y: null});
    }
    if (s.data[s.data.length - 1].x < endTime) {
      s.data.push({x: endTime, y: null});
    }
  });

  // Now create the new graph.
  self.rickshawGraph = new Rickshaw.Graph({
    element: self.graph[0],
    height: Math.max(self.graph.innerHeight(), 100),
    width: Math.max(self.graph.innerWidth() - 80, 200),
    renderer: (self.isStacked() ? "stack" : "line"),
    interpolation: "linear",
    series: self.data,
    min: "auto",
  });

  // Find and set graph's max/min
  if (self.isStacked() === true) {
    // When stacked is toggled
    var max = 0;
    self.data.forEach(function(timeSeries) {
      var currSeriesMax = 0;
      timeSeries.data.forEach(function(dataPoint) {
        if (dataPoint.y > currSeriesMax && dataPoint.y != null) {
          currSeriesMax = dataPoint.y;
        }
      });
      max += currSeriesMax;
    });
    self.rickshawGraph.max = max*1.05;
    self.rickshawGraph.min = 0;
  } else {
    var min = Infinity;
    var max = -Infinity;
    self.data.forEach(function(timeSeries) {
      timeSeries.data.forEach(function(dataPoint) {
        if (dataPoint.y < min && dataPoint.y != null) {
          min = dataPoint.y;
        }
        if (dataPoint.y > max && dataPoint.y != null) {
          max = dataPoint.y;
        }
      });
    });
    var offset = 0.1 * (min === max ? max : Math.abs(max-min));
    self.rickshawGraph.max = max + offset;
    self.rickshawGraph.min = min - offset;
  }

  var xAxis = new Rickshaw.Graph.Axis.Time({ graph: self.rickshawGraph });

  var yAxis = new Rickshaw.Graph.Axis.Y({
    graph: self.rickshawGraph,
    orientation: "left",
    tickFormat: this.formatKMBT,
    element: self.yAxis[0],
  });

  self.rickshawGraph.render();

  var hoverDetail = new Rickshaw.Graph.HoverDetail({
    graph: self.rickshawGraph,
    formatter: function(series, x, y) {
      var date = '<span class="date">' + new Date(x * 1000).toUTCString() + '</span>';
      var swatch = '<span class="detail_swatch" style="background-color: ' + series.color + '"></span>';
      var content = swatch + (series.labels.__name__ || 'value') + ": <strong>" + y + '</strong>';
      return date + '<br>' + content + '<br>' + self.renderLabels(series.labels);
    }
  });

  var legend = new Rickshaw.Graph.Legend({
    element: self.legend[0],
    graph: self.rickshawGraph,
  });

  var highlighter = new Rickshaw.Graph.Behavior.Series.Highlight( {
    graph: self.rickshawGraph,
    legend: legend
  });

  var shelving = new Rickshaw.Graph.Behavior.Series.Toggle({
    graph: self.rickshawGraph,
    legend: legend
  });

  self.handleChange();
};

Prometheus.Graph.prototype.resizeGraph = function() {
  var self = this;
  if (self.rickshawGraph !== null) {
    self.rickshawGraph.configure({
      width: Math.max(self.graph.innerWidth() - 80, 200),
    });
    self.rickshawGraph.render();
  }
};

Prometheus.Graph.prototype.handleGraphResponse = function(json, textStatus) {
  var self = this;
  // Rickshaw mutates passed series data for stacked graphs, so we need to save
  // the original AJAX response in order to re-transform it into series data
  // when the user disables the stacking.
  self.graphJSON = json;
  self.data = self.transformData(json);
  if (self.data.length === 0) {
    self.showError("No datapoints found.");
    return;
  }
  self.graphTab.removeClass("reload");
  self.updateGraph();
};

Prometheus.Graph.prototype.handleConsoleResponse = function(data, textStatus) {
  var self = this;
  self.consoleTab.removeClass("reload");
  self.graphJSON = null;

  var tBody = self.consoleTab.find(".console_table tbody");
  tBody.empty();

  switch(data.resultType) {
  case "vector":
    if (data.result === null || data.result.length === 0) {
      tBody.append("<tr><td colspan='2'><i>no data</i></td></tr>");
      return;
    }
    data.result = self.limitSeries(data.result);
    for (var i = 0; i < data.result.length; i++) {
      var s = data.result[i];
      var tsName = self.metricToTsName(s.metric);
      tBody.append("<tr><td>" + escapeHTML(tsName) + "</td><td>" + s.value[1] + "</td></tr>");
    }
    break;
  case "matrix":
    if (data.result.length === 0) {
      tBody.append("<tr><td colspan='2'><i>no data</i></td></tr>");
      return;
    }
    data.result = self.limitSeries(data.result);
    for (var i = 0; i < data.result.length; i++) {
      var v = data.result[i];
      var tsName = self.metricToTsName(v.metric);
      var valueText = "";
      for (var j = 0; j < v.values.length; j++) {
        valueText += v.values[j][1] + " @" + v.values[j][0] + "<br/>";
      }
      tBody.append("<tr><td>" + escapeHTML(tsName) + "</td><td>" + valueText + "</td></tr>");
    }
    break;
  case "scalar":
    tBody.append("<tr><td>scalar</td><td>" + data.result[1] + "</td></tr>");
    break;
  case "string":
    tBody.append("<tr><td>string</td><td>" + escapeHTML(data.result[1]) + "</td></tr>");
    break;
  default:
    self.showError("Unsupported value type!");
    break;
  }
  self.handleChange();
};

Prometheus.Graph.prototype.remove = function() {
  var self = this;
  $(self.graphHTML).remove();
  self.handleRemove();
  self.handleChange();
};

Prometheus.Graph.prototype.formatKMBT = function(y) {
  var abs_y = Math.abs(y);
  if (abs_y >= 1e24) {
    return (y / 1e24).toString() + "Y";
  } else if (abs_y >= 1e21) {
    return (y / 1e21).toString() + "Z";
  } else if (abs_y >= 1e18) {
    return (y / 1e18).toString() + "E";
  } else if (abs_y >= 1e15) {
    return (y / 1e15).toString() + "P";
  } else if (abs_y >= 1e12) {
    return (y / 1e12).toString() + "T";
  } else if (abs_y >= 1e9) {
    return (y / 1e9).toString() + "G";
  } else if (abs_y >= 1e6) {
    return (y / 1e6).toString() + "M";
  } else if (abs_y >= 1e3) {
    return (y / 1e3).toString() + "k";
  } else if (abs_y >= 1) {
    return y
  } else if (abs_y === 0) {
    return y
  } else if (abs_y <= 1e-24) {
    return (y / 1e-24).toString() + "y";
  } else if (abs_y <= 1e-21) {
    return (y / 1e-21).toString() + "z";
  } else if (abs_y <= 1e-18) {
    return (y / 1e-18).toString() + "a";
  } else if (abs_y <= 1e-15) {
    return (y / 1e-15).toString() + "f";
  } else if (abs_y <= 1e-12) {
    return (y / 1e-12).toString() + "p";
  } else if (abs_y <= 1e-9) {
      return (y / 1e-9).toString() + "n";
  } else if (abs_y <= 1e-6) {
    return (y / 1e-6).toString() + "µ";
  } else if (abs_y <=1e-3) {
    return (y / 1e-3).toString() + "m";
  } else if (abs_y <= 1) {
    return y
  }
}

Prometheus.Graph.prototype.limitSeries = function(result) {
  var self = this;
  var MAX_SERIES_NUM = 10000;
  var message = "<strong>Warning!</strong> Fetched " +
    result.length + " series, but displaying only first " +
    MAX_SERIES_NUM + ".";

  if (result.length > MAX_SERIES_NUM) {
    self.showWarning(message);
    return result.splice(0, MAX_SERIES_NUM);
  }
  return result;
}

/**
 * Page
*/
const pageConfig = {
  allMetrics: [],
  graphs: [],
  queryHistMetrics: JSON.parse(localStorage.getItem('history')) || [],
};

Prometheus.Page = function() {};

Prometheus.Page.prototype.init = function() {
  var graphOptions = this.parseURL();
  if (graphOptions.length === 0) {
    graphOptions.push({});
  }

  graphOptions.forEach(this.addGraph, this);
  $("#add_graph").click(this.addGraph.bind(this, {}));
};

Prometheus.Page.prototype.parseURL = function() {
  if (window.location.search == "") {
    return [];
  }

  var queryParams = window.location.search.substring(1).split('&');
  var queryParamHelper = new Prometheus.Page.QueryParamHelper();
  return queryParamHelper.parseQueryParams(queryParams);
};

Prometheus.Page.prototype.addGraph = function(options) {
  var graph = new Prometheus.Graph(
    $("#graph_container"),
    options,
    this.updateURL.bind(this),
    this.removeGraph.bind(this)
  );

  pageConfig.graphs.push(graph);

  $(window).resize(function() {
    graph.resizeGraph();
  });
};

// NOTE: This needs to be kept in sync with /util/strutil/strconv.go:GraphLinkForExpression
Prometheus.Page.prototype.updateURL = function() {
  var queryString = pageConfig.graphs.map(function(graph, index) {
    var graphOptions = graph.getOptions();
    var queryParamHelper = new Prometheus.Page.QueryParamHelper();
    var queryObject = queryParamHelper.generateQueryObject(graphOptions, index);
    return $.param(queryObject);
  }, this).join("&");

  history.pushState({}, "", "graph?" + queryString);
};

Prometheus.Page.prototype.removeGraph = function(graph) {
  pageConfig.graphs = pageConfig.graphs.filter(function(g) {return g !== graph});
};

Prometheus.Page.QueryParamHelper = function() {};

Prometheus.Page.QueryParamHelper.prototype.parseQueryParams = function(queryParams) {
  var orderedQueryParams = this.filterInvalidParams(queryParams).sort();
  return this.fetchOptionsFromOrderedParams(orderedQueryParams, 0);
};

Prometheus.Page.QueryParamHelper.queryParamFormat = /^g\d+\..+=.+$/;

Prometheus.Page.QueryParamHelper.prototype.filterInvalidParams = function(paramTuples) {
  return paramTuples.filter(function(paramTuple) {
    return Prometheus.Page.QueryParamHelper.queryParamFormat.test(paramTuple);
  });
};

Prometheus.Page.QueryParamHelper.prototype.fetchOptionsFromOrderedParams = function(queryParams, graphIndex) {
  if (queryParams.length == 0) {
    return [];
  }

  var prefixOfThisIndex = this.queryParamPrefix(graphIndex);
  var numberOfParamsForThisGraph = queryParams.filter(function(paramTuple) {
    return paramTuple.startsWith(prefixOfThisIndex);
  }).length;

  if (numberOfParamsForThisGraph == 0) {
    return [];
  }

  var paramsForThisGraph = queryParams.splice(0, numberOfParamsForThisGraph);

  paramsForThisGraph = paramsForThisGraph.map(function(paramTuple) {
    return paramTuple.substring(prefixOfThisIndex.length);
  });

  var options = this.parseQueryParamsOfOneGraph(paramsForThisGraph);
  var optionAccumulator = this.fetchOptionsFromOrderedParams(queryParams, graphIndex + 1);
  optionAccumulator.unshift(options);

  return optionAccumulator;
};

Prometheus.Page.QueryParamHelper.prototype.parseQueryParamsOfOneGraph = function(queryParams) {
  var options = {};
  queryParams.forEach(function(tuple) {
    var optionNameAndValue = tuple.split('=');
    var optionName = optionNameAndValue[0];

    var optionValue = decodeURIComponent(optionNameAndValue[1].replace(/\+/g, " "));

    if (optionName == "tab") {
      optionValue = parseInt(optionValue); // tab is integer
    }

    options[optionName] = optionValue;
  });

  return options;
};

Prometheus.Page.QueryParamHelper.prototype.queryParamPrefix = function(index) {
  return "g" + index + ".";
};

Prometheus.Page.QueryParamHelper.prototype.generateQueryObject = function(graphOptions, index) {
  var prefix = this.queryParamPrefix(index);
  var queryObject = {};
  Object.keys(graphOptions).forEach(function(key) {
    queryObject[prefix + key] = graphOptions[key];
  });
  return queryObject;
};

// These two methods (isDeprecatedGraphURL and redirectToMigratedURL)
// are added only for backward compatibility to old query format.
function isDeprecatedGraphURL() {
  if (window.location.hash.length == 0) {
    return false;
  }

  var decodedFragment = decodeURIComponent(window.location.hash);
  try {
      JSON.parse(decodedFragment.substr(1)); // drop the hash #
  } catch (e) {
      return false;
  }
  return true;
}

function redirectToMigratedURL() {
  var decodedFragment = decodeURIComponent(window.location.hash);
  var graphOptions = JSON.parse(decodedFragment.substr(1)); // drop the hash #
  var queryObject = {};

  graphOptions.map(function(options, index){
    var prefix = "g" + index + ".";
    Object.keys(options).forEach(function(key) {
      queryObject[prefix + key] = options[key];
    });
  });
  var query = $.param(queryObject);
  window.location = PATH_PREFIX + "/graph?" + query;
}

/**
 * Query History helper functions
 * **/
const queryHistory = {
  isEnabled: function() {
    return JSON.parse(localStorage.getItem('enable-query-history'))
  },

  bindHistoryEvents: function(graph) {
    const targetEl = $('div.query-history');
    const icon = $(targetEl).children('i');
    targetEl.off('click');

    if (queryHistory.isEnabled()) {
      this.toggleOn(targetEl);
    }

    targetEl.on('click', function() {
      if (icon.hasClass('glyphicon-unchecked')) {
        queryHistory.toggleOn(targetEl);
      } else if (icon.hasClass('glyphicon-check')) {
        queryHistory.toggleOff(targetEl);
      }
    });
  },

  handleHistory: function(graph) {
    const query = graph.expr.val();
    const isSimpleMetric = pageConfig.allMetrics.indexOf(query) !== -1;
    if (isSimpleMetric) {
      return;
    }

    let parsedQueryHistory = JSON.parse(localStorage.getItem('history'));
    const hasStoredQuery = parsedQueryHistory.indexOf(query) !== -1;
    if (hasStoredQuery) {
      parsedQueryHistory.splice(parsedQueryHistory.indexOf(query), 1);
    }

    parsedQueryHistory.push(query);
    const queryCount = parsedQueryHistory.length;
    parsedQueryHistory = parsedQueryHistory.slice(queryCount - 50, queryCount);

    localStorage.setItem('history', JSON.stringify(parsedQueryHistory));
    pageConfig.queryHistMetrics = parsedQueryHistory;
    if (queryHistory.isEnabled()) {
      this.updateTypeaheadMetricSet(pageConfig.queryHistMetrics.concat(pageConfig.allMetrics));
    }
  },

  toggleOn: function(targetEl) {
    this.updateTypeaheadMetricSet(pageConfig.queryHistMetrics.concat(pageConfig.allMetrics));

    $(targetEl).children('i').removeClass('glyphicon-unchecked').addClass('glyphicon-check');
    targetEl.addClass('is-checked');
    localStorage.setItem('enable-query-history', true);
  },

  toggleOff: function(targetEl) {
    this.updateTypeaheadMetricSet(pageConfig.allMetrics);

    $(targetEl).children('i').removeClass('glyphicon-check').addClass('glyphicon-unchecked');
    targetEl.removeClass('is-checked');
    localStorage.setItem('enable-query-history', false);
  },

  updateTypeaheadMetricSet: function(metricSet) {
    pageConfig.graphs.forEach(function(graph) {
      if (graph.expr.data('typeahead')) {
        graph.expr.data('typeahead').source = metricSet;
      }
    });
  }
};

// Defers invocation of fn for waitPeriod if returned function is called repeatedly
function debounce(fn, waitPeriod) {
  var timeout;
  return function() {
    clearTimeout(timeout);
    var args = arguments;
    var scope = this;
    function invokeFn() {
      return fn.apply(scope, args);
    }
    timeout = setTimeout(invokeFn, waitPeriod);
  };
}

function escapeHTML(string) {
  var entityMap = {
    "&": "&amp;",
    "<": "&lt;",
    ">": "&gt;",
    '"': '&quot;',
    "'": '&#39;',
    "/": '&#x2F;'
  };

  return String(string).replace(/[&<>"'\/]/g, function (s) {
    return entityMap[s];
  });
}

function init() {
  $.ajaxSetup({
    cache: false
  });

  $.ajax({
    url: PATH_PREFIX + "/static/js/graph/graph_template.handlebar?v=" + BUILD_VERSION,
    success: function(data) {

      graphTemplate = data;
      Mustache.parse(data);
      if (isDeprecatedGraphURL()) {
        redirectToMigratedURL();
      } else {
        var Page = new Prometheus.Page();
        Page.init();
      }
    }
  });
}

$(init);
