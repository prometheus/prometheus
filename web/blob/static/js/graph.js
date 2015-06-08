var Prometheus = Prometheus || {};
var graphs = [];
var graphTemplate;

var SECOND = 1000;

Handlebars.registerHelper('pathPrefix', function() { return PATH_PREFIX; });

Prometheus.Graph = function(element, options) {
  this.el = element;
  this.options = options;
  this.changeHandler = null;
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
  self.options["id"] = self.id;
  self.options["range_input"] = self.options["range_input"] || "1h";
  if (self.options["tab"] === undefined) {
    self.options["tab"] = 1;
  }

  // Draw graph controls and container from Handlebars template.

  var graphHtml = graphTemplate(self.options);
  self.el.append(graphHtml);

  // Get references to all the interesting elements in the graph container and
  // bind event handlers.
  var graphWrapper = self.el.find("#graph_wrapper" + self.id);
  self.queryForm = graphWrapper.find(".query_form");
  self.expr = graphWrapper.find("input[name=expr]");
  self.expr.change(storeGraphOptionsInURL);
  self.rangeInput = self.queryForm.find("input[name=range_input]");
  self.stackedBtn = self.queryForm.find(".stacked_btn");
  self.stacked = self.queryForm.find("input[name=stacked]");
  self.insertMetric = self.queryForm.find("select[name=insert_metric]");
  self.refreshInterval = self.queryForm.find("select[name=refresh]");

  self.consoleTab = graphWrapper.find(".console");
  self.graphTab   = graphWrapper.find(".graph_container");

  self.tabs = graphWrapper.find("a[data-toggle='tab']");
  self.tabs.eq(self.options["tab"]).tab("show");
  self.tabs.on("shown.bs.tab", function(e) {
    var target = $(e.target);
    self.options["tab"] = target.parent().index();
    storeGraphOptionsInURL();
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
  self.graphArea = graphWrapper.find(".graph_area");
  self.graph = self.graphArea.find(".graph");
  self.yAxis = self.graphArea.find(".y_axis");
  self.legend = graphWrapper.find(".legend");
  self.spinner = graphWrapper.find(".spinner");
  self.evalStats = graphWrapper.find(".eval_stats");

  self.endDate = graphWrapper.find("input[name=end_input]");
  self.endDate.datetimepicker({
    language: 'en',
    pickSeconds: false,
  });
  if (self.options["end_input"]) {
    self.endDate.data('datetimepicker').setValue(self.options["end_input"]);
  }
  self.endDate.change(function() { self.submitQuery() });
  self.refreshInterval.change(function() { self.updateRefresh() });

  self.isStacked = function() {
    return self.stacked.val() === '1';
  };

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
    self.submitQuery();
    return false;
  });
  self.spinner.hide();

  self.queryForm.find("button[name=inc_range]").click(function() { self.increaseRange(); });
  self.queryForm.find("button[name=dec_range]").click(function() { self.decreaseRange(); });

  self.queryForm.find("button[name=inc_end]").click(function() { self.increaseEnd(); });
  self.queryForm.find("button[name=dec_end]").click(function() { self.decreaseEnd(); });

  self.insertMetric.change(function() {
    self.expr.selection("replace", {text: self.insertMetric.val(), mode: "before"});
    self.expr.focus(); // refocusing
  });

  self.populateInsertableMetrics();

  if (self.expr.val()) {
    self.submitQuery();
  }
};

Prometheus.Graph.prototype.populateInsertableMetrics = function() {
  var self = this;
  $.ajax({
      method: "GET",
      url: PATH_PREFIX + "/api/metrics",
      dataType: "json",
      success: function(json, textStatus) {
        var availableMetrics = [];
        for (var i = 0; i < json.length; i++) {
          self.insertMetric[0].options.add(new Option(json[i], json[i]));
          availableMetrics.push(json[i]);
        }
        self.expr.typeahead({
          source: availableMetrics,
          items: "all"
        });
        // This needs to happen after attaching the typeahead plugin, as it
        // otherwise breaks the typeahead functionality.
        self.expr.focus();
      },
      error: function() {
        self.showError("Error loading available metrics!");
      },
  });
};

Prometheus.Graph.prototype.onChange = function(handler) {
  this.changeHandler = handler;
};

Prometheus.Graph.prototype.getOptions = function() {
  var self = this;
  var options = {};

  var optionInputs = [
    "expr",
    "range_input",
    "end_input",
    "step_input",
    "stacked"
  ];

  self.queryForm.find("input").each(function(index, element) {
      var name = element.name;
      if ($.inArray(name, optionInputs) >= 0) {
        options[name] = element.value;
      }
  });
  options["tab"] = self.options["tab"];
  return options;
};

Prometheus.Graph.prototype.parseDuration = function(rangeText) {
  var rangeRE = new RegExp("^([0-9]+)([ywdhms]+)$");
  var matches = rangeText.match(rangeRE);
  if (!matches) { return };
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
    return null;
  }
  return self.endDate.data('datetimepicker').getDate().getTime();
};

Prometheus.Graph.prototype.getOrSetEndDate = function() {
  var self = this;
  var date = self.getEndDate();
  if (date) {
    return date;
  }
  date = new Date();
  self.setEndDate(date);
  return date;
}

Prometheus.Graph.prototype.setEndDate = function(date) {
  var self = this;
  self.endDate.data('datetimepicker').setValue(date);
};

Prometheus.Graph.prototype.increaseEnd = function() {
  var self = this;
  self.setEndDate(new Date(self.getOrSetEndDate() + self.parseDuration(self.rangeInput.val()) * 1000/2 )) // increase by 1/2 range & convert ms in s
  self.submitQuery();
};

Prometheus.Graph.prototype.decreaseEnd = function() {
  var self = this;
  self.setEndDate(new Date(self.getOrSetEndDate() - self.parseDuration(self.rangeInput.val()) * 1000/2 ))
  self.submitQuery();
};

Prometheus.Graph.prototype.submitQuery = function() {
  var self = this;
  self.clearError();
  if (!self.expr.val()) {
    return;
  }

  self.spinner.show();
  self.evalStats.empty();

  var startTime = new Date().getTime();

  var rangeSeconds = self.parseDuration(self.rangeInput.val());
  self.queryForm.find("input[name=range]").val(rangeSeconds);
  var resolution = self.queryForm.find("input[name=step_input]").val() || Math.max(Math.floor(rangeSeconds / 250), 1);
  self.queryForm.find("input[name=step]").val(resolution);
  var endDate = self.getEndDate() / 1000;
  self.queryForm.find("input[name=end]").val(endDate);

  if (self.queryXhr) {
    self.queryXhr.abort();
  }
  var url;
  var success;
  if (self.options["tab"] === 0) {
    url  = self.queryForm.attr("action");
    success = function(json, textStatus) { self.handleGraphResponse(json, textStatus); };
  } else {
    url  = PATH_PREFIX + "/api/query";
    success = function(text, textStatus) { self.handleConsoleResponse(text, textStatus); };
  }

  self.queryXhr = $.ajax({
      method: self.queryForm.attr("method"),
      url: url,
      dataType: "json",
      data: self.queryForm.serialize(),
      success: success,
      error: function(xhr, resp) {
        if (resp != "abort") {
          self.showError("Error executing query: " + resp);
        }
      },
      complete: function() {
        var duration = new Date().getTime() - startTime;
        self.evalStats.html("Load time: " + duration + "ms <br /> Resolution: " + resolution + "s");
        self.spinner.hide();
      }
  });
};

Prometheus.Graph.prototype.showError = function(msg) {
  var self = this;
  self.error.text(msg);
  self.error.show();
}

Prometheus.Graph.prototype.clearError = function(msg) {
  var self = this;
  self.error.text('');
  self.error.hide();
}

Prometheus.Graph.prototype.updateRefresh = function() {
  var self = this;

  if (self.timeoutID) {
    window.clearTimeout(self.timeoutID);
  }

  interval = self.parseDuration(self.refreshInterval.val());
  if (!interval) { return };

  self.timeoutID = window.setTimeout(function() {
    self.submitQuery();
    self.updateRefresh();
  }, interval * SECOND);
}

Prometheus.Graph.prototype.renderLabels = function(labels) {
  var labelStrings = [];
  for (label in labels) {
    if (label != "__name__") {
      labelStrings.push("<strong>" + label + "</strong>: " + escapeHTML(labels[label]));
    }
  }
  return labels = "<div class=\"labels\">" + labelStrings.join("<br>") + "</div>";
}

Prometheus.Graph.prototype.metricToTsName = function(labels) {
  var tsName = (labels["__name__"] || '') + "{";
  var labelStrings = [];
   for (label in labels) {
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
    return null
  }
  return val;
};

Prometheus.Graph.prototype.transformData = function(json) {
  var self = this;
  var palette = new Rickshaw.Color.Palette();
  if (json.type != "matrix") {
    self.showError("Result is not of matrix type! Please enter a correct expression.");
    return [];
  }
  var data = json.value.map(function(ts) {
    return {
      name: escapeHTML(self.metricToTsName(ts.metric)),
      labels: ts.metric,
      data: ts.values.map(function(value) {
        return {
          x: value[0],
          y: self.parseValue(value[1])
        }
      }),
      color: palette.color()
    };
  });
  Rickshaw.Series.zeroFill(data);
  return data;
};

Prometheus.Graph.prototype.updateGraph = function() {
  var self = this;
  if (self.data.length == 0) { return; }

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

  var xAxis = new Rickshaw.Graph.Axis.Time({ graph: self.rickshawGraph });

  var yAxis = new Rickshaw.Graph.Axis.Y({
    graph: self.rickshawGraph,
    orientation: "left",
    tickFormat: Rickshaw.Fixtures.Number.formatKMBT,
    element: self.yAxis[0],
  });

  self.rickshawGraph.render();

  var hoverDetail = new Rickshaw.Graph.HoverDetail({
    graph: self.rickshawGraph,
    formatter: function(series, x, y) {
      var swatch = '<span class="detail_swatch" style="background-color: ' + series.color + '"></span>';
      var content = swatch + (series.labels["__name__"] || 'value') + ": <strong>" + y + '</strong><br>';
      return content + self.renderLabels(series.labels);
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

  self.changeHandler();
};

Prometheus.Graph.prototype.resizeGraph = function() {
  var self = this;
  if (self.rickshawGraph != null) {
    self.rickshawGraph.configure({
      width: Math.max(self.graph.innerWidth() - 80, 200),
    });
    self.rickshawGraph.render();
  }
}

Prometheus.Graph.prototype.handleGraphResponse = function(json, textStatus) {
  var self = this
  if (json.type == "error") {
    self.showError(json.value);
    return;
  }
  // Rickshaw mutates passed series data for stacked graphs, so we need to save
  // the original AJAX response in order to re-transform it into series data
  // when the user disables the stacking.
  self.graphJSON = json;
  self.data = self.transformData(json);
  if (self.data.length == 0) {
    self.showError("No datapoints found.");
    return;
  }
  self.graphTab.removeClass("reload");
  self.updateGraph();
}

Prometheus.Graph.prototype.handleConsoleResponse = function(data, textStatus) {
  var self = this;
  self.consoleTab.removeClass("reload");
  self.graphJSON = null;

  var tBody = self.consoleTab.find(".console_table tbody");
  tBody.empty();

  switch(data.type) {
  case "vector":
    if (data.value.length === 0) {
      tBody.append("<tr><td colspan='2'><i>no data</i></td></tr>");
      return;
    }
    for (var i = 0; i < data.value.length; i++) {
      var v = data.value[i];
      var tsName = self.metricToTsName(v.metric);
      tBody.append("<tr><td>" + escapeHTML(tsName) + "</td><td>" + v.value + "</td></tr>");
    }
    break;
  case "matrix":
    if (data.value.length === 0) {
      tBody.append("<tr><td colspan='2'><i>no data</i></td></tr>");
      return;
    }
    for (var i = 0; i < data.value.length; i++) {
      var v = data.value[i];
      var tsName = self.metricToTsName(v.metric);
      var valueText = "";
      for (var j = 0; j < v.values.length; j++) {
        valueText += v.values[j][1] + " @" + v.values[j][0] + "<br/>";
      }
      tBody.append("<tr><td>" + escapeHTML(tsName) + "</td><td>" + valueText + "</td></tr>")
    }
    break;
  case "scalar":
    tBody.append("<tr><td>scalar</td><td>" + data.value + "</td></tr>");
    break;
  case "error":
    self.showError(data.value);
    break;
  default:
    self.showError("Unsupported value type!");
    break;
  }
}

function parseGraphOptionsFromURL() {
  var hashOptions = window.location.hash.slice(1);
  if (!hashOptions) {
    return [];
  }
  var optionsJSON = decodeURIComponent(window.location.hash.slice(1));
  options = JSON.parse(optionsJSON);
  return options;
}

// NOTE: This needs to be kept in sync with rules/helpers.go:GraphLinkForExpression!
function storeGraphOptionsInURL() {
  var allGraphsOptions = [];
  for (var i = 0; i < graphs.length; i++) {
    allGraphsOptions.push(graphs[i].getOptions());
  }
  var optionsJSON = JSON.stringify(allGraphsOptions);
  window.location.hash = encodeURIComponent(optionsJSON);
}

function addGraph(options) {
  var graph = new Prometheus.Graph($("#graph_container"), options);
  graphs.push(graph);
  graph.onChange(function() {
    storeGraphOptionsInURL();
  });
  $(window).resize(function() {
    graph.resizeGraph();
  });
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
    url: PATH_PREFIX + "/static/js/graph_template.handlebar",
    success: function(data) {
      graphTemplate = Handlebars.compile(data);
      var options = parseGraphOptionsFromURL();
      if (options.length == 0) {
        options.push({});
      }
      for (var i = 0; i < options.length; i++) {
        addGraph(options[i]);
      }
      $("#add_graph").click(function() { addGraph({}); });
    }
  })
}

$(init);
