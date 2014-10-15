// Graph options we might want:
//  Grid off/on
//  Stacked off/on
//  Area off/on
//  Legend position
//  Short link
//  Graph title
//  Palette
//  Background
//  Enable tooltips
//  width/height
//  Axis options
//  Y-Range min/max
//  (X-Range min/max)
//  X-Axis format
//  Y-Axis format
//  Y-Axis title
//  X-Axis title
//  Log scale

var Prometheus = Prometheus || {};
var graphs = [];
var graphTemplate;

var SECOND = 1000;

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
  self.options["stacked_checked"] = self.options["stacked"] ? "checked" : "";
  self.options["tab"] = self.options["tab"] || 0;

  // Draw graph controls and container from Handlebars template.

  var graphHtml = graphTemplate(self.options);
  self.el.append(graphHtml);

  // Get references to all the interesting elements in the graph container and
  // bind event handlers.
  var graphWrapper = self.el.find("#graph_wrapper" + self.id);
  self.queryForm = graphWrapper.find(".query_form");
  self.expr = graphWrapper.find("input[name=expr]");
  self.rangeInput = self.queryForm.find("input[name=range_input]");
  self.stacked = self.queryForm.find("input[name=stacked]");
  self.insertMetric = self.queryForm.find("select[name=insert_metric]");
  self.refreshInterval = self.queryForm.find("select[name=refresh]");

  self.consoleTab = graphWrapper.find(".console");
  self.graphTab   = graphWrapper.find(".graph_container");
  self.tabs = graphWrapper.find(".tabs");
  self.tab  = $(self.tabs.find("> div")[self.options["tab"]]); // active tab

  self.tabs.tabs({
    active: self.options["tab"],
    activate: function(e, ui) {
      storeGraphOptionsInUrl();
      self.tab = ui.newPanel
      if (self.tab.hasClass("reload")) { // reload if flagged with class "reload"
        self.submitQuery();
      }
    }
  });

  // Return moves focus back to expr instead of submitting.
  self.insertMetric.bind("keydown", "return", function(e) {
    self.expr.focus();
    self.expr.val(self.expr.val());

    return e.preventDefault();
  })

  self.graph = graphWrapper.find(".graph");
  self.yAxis = graphWrapper.find(".y_axis");
  self.legend = graphWrapper.find(".legend");
  self.spinner = graphWrapper.find(".spinner");
  self.evalStats = graphWrapper.find(".eval_stats");

  self.endDate = graphWrapper.find("input[name=end_input]");
  if (self.options["end_input"]) {
    self.endDate.appendDtpicker({"current": self.options["end_input"]});
  } else {
    self.endDate.appendDtpicker();
    self.endDate.val("");
  }
  self.endDate.change(function() { self.submitQuery() });
  self.refreshInterval.change(function() { self.updateRefresh() });

  self.stacked.change(function() { self.updateGraph(); });
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

  self.expr.focus(); // TODO: move to external Graph method.

  self.populateInsertableMetrics();

  if (self.expr.val()) {
    self.submitQuery();
  }
};

Prometheus.Graph.prototype.populateInsertableMetrics = function() {
  var self = this;
  $.ajax({
      method: "GET",
      url: "/api/metrics",
      dataType: "json",
      success: function(json, textStatus) {
        var availableMetrics = [];
        for (var i = 0; i < json.length; i++) {
          self.insertMetric[0].options.add(new Option(json[i], json[i]));
          availableMetrics.push(json[i]);
        }
        self.expr.autocomplete({source: availableMetrics});
      },
      error: function() {
        alert("Error loading available metrics!");
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
        if (name == "stacked") {
          options[name] = element.checked;
        } else {
          options[name] = element.value;
        }
      }
  });
  options["tab"] = self.tabs.tabs("option", "active");
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
  return new Date(self.endDate.val()).getTime();
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
  dateString = date.getFullYear() + "-" + (date.getMonth()+1) + "-" + date.getDate() + " " +
               date.getHours() + ":" + date.getMinutes();
  self.endDate.val("");
  self.endDate.appendDtpicker({"current": dateString});
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
  if (self.tab[0] == self.graphTab[0]) {
    url  = self.queryForm.attr("action");
    success = function(json, textStatus) { self.handleGraphResponse(json, textStatus); };
  } else {
    url  = "/api/query";
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
          alert("Error executing query: " + resp);
        }
      },
      complete: function() {
        var duration = new Date().getTime() - startTime;
        self.evalStats.html("Load time: " + duration + "ms, resolution: " + resolution + "s");
        self.spinner.hide();
      }
  });
};

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
      labelStrings.push("<strong>" + label + "</strong>: " + labels[label]);
    }
  }
  return labels = "<div class=\"labels\">" + labelStrings.join("<br>") + "</div>";
}

Prometheus.Graph.prototype.metricToTsName = function(labels) {
  var tsName = labels["__name__"] + "{";
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
  if (value == "NaN" || value == "Inf" || value == "-Inf") {
    return 0; // TODO: what should we really do here?
  } else {
    return parseFloat(value);
  }
};

Prometheus.Graph.prototype.transformData = function(json) {
  self = this;
  var palette = new Rickshaw.Color.Palette();
  if (json.Type != "matrix") {
    alert("Result is not of matrix type! Please enter a correct expression.");
    return [];
  }
  var data = json.Value.map(function(ts) {
    return {
      name: self.metricToTsName(ts.Metric),
      labels: ts.Metric,
      data: ts.Values.map(function(value) {
        return {
          x: value.Timestamp,
          y: self.parseValue(value.Value)
        }
      }),
      color: palette.color()
    };
  });
  Rickshaw.Series.zeroFill(data);
  return data;
};

Prometheus.Graph.prototype.showGraph = function() {
  var self = this;
  self.rickshawGraph = new Rickshaw.Graph({
    element: self.graph[0],
    height: Math.max(self.graph.innerHeight(), 100),
    width: Math.max(self.graph.innerWidth() - 80, 200),
    renderer: (self.stacked.is(":checked") ? "stack" : "line"),
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
};

Prometheus.Graph.prototype.updateGraph = function(reloadGraph) {
  var self = this;
  if (self.data.length == 0) { return; }
  self.legend.empty();
  if (self.rickshawGraph == null || reloadGraph) {
    self.yAxis.empty();
    self.graph.empty();
    self.showGraph();
  } else {
    self.rickshawGraph.configure({
      renderer: (self.stacked.is(":checked") ? "stack" : "line"),
      interpolation: "linear",
      series: self.data
    });
    self.rickshawGraph.render();
  }

  var hoverDetail = new Rickshaw.Graph.HoverDetail({
    graph: self.rickshawGraph,
    formatter: function(series, x, y) {
      var swatch = '<span class="detail_swatch" style="background-color: ' + series.color + '"></span>';
      var content = swatch + series.labels["__name__"] + ": <strong>" + y + '</strong><br>';
      return content + self.renderLabels(series.labels);
    },
    onRender: function() {
      var width = this.graph.width;
      var element = $(this.element);

      $(".x_label", element).each(function() {
        if ($(this).outerWidth() + element.offset().left > width) {
          $(this).addClass("flipped");
        } else {
          $(this).removeClass("flipped");
        }
      })

      $(".item", element).each(function() {
        if ($(this).outerWidth() + element.offset().left > width) {
          $(this).addClass("flipped");
        } else {
          $(this).removeClass("flipped");
        }
      })
    },
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
  if (json.Type == "error") {
    alert(json.Value);
    return;
  }
  self.data = self.transformData(json);
  if (self.data.length == 0) {
    alert("No datapoints found.");
    return;
  }
  self.graphTab.removeClass("reload");
  self.updateGraph(true);
}

Prometheus.Graph.prototype.handleConsoleResponse = function(data, textStatus) {
  var self = this;
  self.consoleTab.removeClass("reload");

  var tBody = self.consoleTab.find(".console_table tbody");
  tBody.empty();

  switch(data.Type) {
  case "vector":
    for (var i = 0; i < data.Value.length; i++) {
      var v = data.Value[i];
      var tsName = self.metricToTsName(v.Metric);
      tBody.append("<tr><td>" + tsName + "</td><td>" + v.Value + "</td></tr>")
    }
    break;
  case "matrix":
    for (var i = 0; i < data.Value.length; i++) {
      var v = data.Value[i];
      var tsName = self.metricToTsName(v.Metric);
      var valueText = "";
      for (var j = 0; j < v.Values.length; j++) {
        valueText += v.Values[j].Value + " @" + v.Values[j].Timestamp + "<br/>";
      }
      tBody.append("<tr><td>" + tsName + "</td><td>" + valueText + "</td></tr>")
    }
    break;
  case "scalar":
    tBody.append("<tr><td>scalar</td><td>" + data.Value + "</td></tr>");
    break;
  case "error":
    alert(data.Value);
    break;
  default:
    alert("Unsupported value type!");
    break;
  }
}

function parseGraphOptionsFromUrl() {
  var hashOptions = window.location.hash.slice(1);
  if (!hashOptions) {
    return [];
  }
  var optionsJSON = decodeURIComponent(window.location.hash.slice(1));
  options = JSON.parse(optionsJSON);
  return options;
}

// NOTE: This needs to be kept in sync with rules/helpers.go:GraphLinkForExpression!
function storeGraphOptionsInUrl() {
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
    storeGraphOptionsInUrl();
  });
  $(window).resize(function() {
    graph.resizeGraph();
  });
}

function init() {
  $.ajaxSetup({
    cache: false
  });

  $.ajax({
    url: "/static/js/graph_template.handlebar",
    success: function(data) {
      graphTemplate = Handlebars.compile(data);
      var options = parseGraphOptionsFromUrl();
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
