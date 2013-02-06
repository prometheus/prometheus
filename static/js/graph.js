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
  self.options['id'] = self.id;
  self.options['range_input'] = self.options['range_input'] || "1h";
  self.options['stacked_checked'] = self.options['stacked'] ? "checked" : "";

  // Draw graph controls and container from Handlebars template.
  var source = $("#graph_template").html();
  var template = Handlebars.compile(source);
  var graphHtml = template(self.options);
  self.el.append(graphHtml);

  // Get references to all the interesting elements in the graph container and
  // bind event handlers.
  var graphWrapper = self.el.find("#graph_wrapper" + self.id);
  self.queryForm = graphWrapper.find(".query_form");
  self.expr = graphWrapper.find("input[name=expr]");
  self.rangeInput = self.queryForm.find("input[name=range_input]");
  self.stacked = self.queryForm.find("input[name=stacked]");
  self.insertMetric = self.queryForm.find("select[name=insert_metric]");

  self.graph = graphWrapper.find(".graph");
  self.legend = graphWrapper.find(".legend");
  self.spinner = graphWrapper.find(".spinner");
  self.evalStats = graphWrapper.find(".eval_stats");

  self.stacked.change(function() { self.updateGraph(); });
  self.queryForm.submit(function() { self.submitQuery(); return false; });
  self.spinner.hide();

  self.queryForm.find("input[name=inc_range]").click(function() { self.increaseRange(); });
  self.queryForm.find("input[name=dec_range]").click(function() { self.decreaseRange(); });
  self.insertMetric.change(function() {
      self.expr.val(self.expr.val() + self.insertMetric.val());
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
        for (var i = 0; i < json.length; i++) {
          self.insertMetric[0].options.add(new Option(json[i], json[i]));
        }
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
    "end",
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
  return options;
};

Prometheus.Graph.prototype.parseRange = function(rangeText) {
  var rangeRE = new RegExp("^([0-9]+)([ywdhms]+)$");
  var matches = rangeText.match(rangeRE);
  if (matches.length != 3) {
    return 60;
  }
  var value = parseInt(matches[1]);
  var unit = matches[2];
  return value * Prometheus.Graph.timeFactors[unit];
};

Prometheus.Graph.prototype.increaseRange = function() {
  var self = this;
  var rangeSeconds = self.parseRange(self.rangeInput.val());
  for (var i = 0; i < Prometheus.Graph.stepValues.length; i++) {
    if (rangeSeconds < self.parseRange(Prometheus.Graph.stepValues[i])) {
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
  var rangeSeconds = self.parseRange(self.rangeInput.val());
  for (var i = Prometheus.Graph.stepValues.length - 1; i >= 0; i--) {
    if (rangeSeconds > self.parseRange(Prometheus.Graph.stepValues[i])) {
      self.rangeInput.val(Prometheus.Graph.stepValues[i]);
      if (self.expr.val()) {
        self.submitQuery();
      }
      return;
    }
  }
};

Prometheus.Graph.prototype.submitQuery = function() {
  var self = this;

  self.spinner.show();
  self.evalStats.empty();

  var startTime = new Date().getTime();

  var rangeSeconds = self.parseRange(self.rangeInput.val());
  self.queryForm.find("input[name=range]").val(rangeSeconds);
  var resolution = self.queryForm.find("input[name=step_input]").val() || Math.max(Math.floor(rangeSeconds / 250), 1);
  self.queryForm.find("input[name=step]").val(resolution);

  $.ajax({
      method: self.queryForm.attr("method"),
      url: self.queryForm.attr("action"),
      dataType: "json",
      data: self.queryForm.serialize(),
      success: function(json, textStatus) {
        if (json.Type == "error") {
          alert(json.Value);
          return;
        }
        self.data = self.transformData(json);
        if (self.data.length == 0) {
          alert("No datapoints found.");
          return;
        }
        self.updateGraph(true);
      },
      error: function() {
        alert("Error executing query!");
      },
      complete: function() {
        var duration = new Date().getTime() - startTime;
        self.evalStats.html("Load time: " + duration + "ms, resolution: " + resolution + "s");
        self.spinner.hide();
      }
  });
};

Prometheus.Graph.prototype.metricToTsName = function(labels) {
  var tsName = labels["name"] + "{";
  var labelStrings = [];
  for (label in labels) {
    if (label != "name") {
      labelStrings.push(label + "='" + labels[label] + "'");
    }
  }
  tsName += labelStrings.join(",") + "}";
  return tsName;
};

Prometheus.Graph.prototype.parseValue = function(value) {
  if (value == "NaN" || value == "Inf" || value == "-Inf") {
    return 0; // TODO: what should we really do here?
  } else {
    return parseFloat(value)
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
    height: 800,
    renderer: (self.stacked.is(":checked") ? "stack" : "line"),
    interpolation: "linear",
    series: self.data
  });

  var xAxis = new Rickshaw.Graph.Axis.Time({ graph: self.rickshawGraph });

  var yAxis = new Rickshaw.Graph.Axis.Y({
    graph: self.rickshawGraph,
    tickFormat: Rickshaw.Fixtures.Number.formatKMBT,
  });

  self.rickshawGraph.render();
};

Prometheus.Graph.prototype.updateGraph = function(reloadGraph) {
  var self = this;
  if (self.data.length == 0) { return; }
  self.legend.empty();
  if (self.rickshawGraph == null || reloadGraph) {
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
    graph: self.rickshawGraph
  });

  var legend = new Rickshaw.Graph.Legend({
    element: self.legend[0],
    graph: self.rickshawGraph
  });

  var shelving = new Rickshaw.Graph.Behavior.Series.Toggle({
    graph: self.rickshawGraph,
    legend: legend
  });

  self.changeHandler();
};

function parseGraphOptionsFromUrl() {
  var hashOptions = window.location.hash.slice(1);
  if (!hashOptions) {
    return [];
  }
  var optionsJSON = decodeURIComponent(window.location.hash.slice(1));
  return JSON.parse(optionsJSON);
}

function storeGraphOptionsInUrl(options) {
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
}

function init() {
  jQuery.ajaxSetup({
    cache: false
  });

  var options = parseGraphOptionsFromUrl();
  if (options.length == 0) {
    options.push({});
  }
  for (var i = 0; i < options.length; i++) {
    addGraph(options[i]);
  }

  $("#add_graph").click(function() { addGraph({}); });
}

$(init);
