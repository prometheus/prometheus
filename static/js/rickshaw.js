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

var graph = null;
var data = [];

var timeFactors = {
  "y": 60 * 60 * 24 * 365,
  "w": 60 * 60 * 24 * 7,
  "d": 60 * 60 * 24,
  "h": 60 * 60,
  "m": 60,
  "s": 1
};

var steps = ["1s", "10s", "1m", "5m", "15m", "30m", "1h", "2h", "6h", "12h",
             "1d", "2d", "1w", "2w", "4w", "8w", "1y", "2y"];

function parseRange(rangeText) {
  var rangeRE = new RegExp("^([0-9]+)([ywdhms]+)$");
  var matches = rangeText.match(rangeRE);
  if (matches.length != 3) {
    return 60;
  }
  var value = parseInt(matches[1]);
  var unit = matches[2];
  return value * timeFactors[unit];
}

function increaseRange() {
  var rangeSeconds = parseRange($("#range_input").val());
  for (var i = 0; i < steps.length; i++) {
    if (rangeSeconds < parseRange(steps[i])) {
      $("#range_input").val(steps[i]);
      submitQuery();
      return;
    }
  }
}

function decreaseRange() {
  var rangeSeconds = parseRange($("#range_input").val());
  for (var i = steps.length - 1; i >= 0; i--) {
    if (rangeSeconds > parseRange(steps[i])) {
      $("#range_input").val(steps[i]);
      submitQuery();
      return;
    }
  }
}

function submitQuery() {
  $("#spinner").show();
  $("#eval_stats").empty();

  var form = $("#query_form");
  var startTime = new Date().getTime();

  var rangeSeconds = parseRange($("#range_input").val());
  $("#range").val(rangeSeconds);
  var resolution = $("#step_input").val() || Math.max(Math.floor(rangeSeconds / 250), 1);
  $("#step").val(resolution);

  $.ajax({
      method: form.attr("method"),
      url: form.attr("action"),
      dataType: "json",
      data: form.serialize(),
      success: function(json, textStatus) {
        if (json.Type == "error") {
          alert(json.Value);
          return;
        }
        data = transformData(json);
        if (data.length == 0) {
          alert("No datapoints found.");
          return;
        }
        updateGraph(true);
      },
      error: function() {
        alert("Error executing query!");
      },
      complete: function() {
        var duration = new Date().getTime() - startTime;
        $("#eval_stats").html("Load time: " + duration + "ms, resolution: " + resolution + "s");
        $("#spinner").hide();
      }
  });
  return false;
}

function metricToTsName(labels) {
  var tsName = labels["name"] + "{";
  var labelStrings = [];
  for (label in labels) {
    if (label != "name") {
      labelStrings.push(label + "='" + labels[label] + "'");
    }
  }
  tsName += labelStrings.join(",") + "}";
  return tsName;
}

function parseValue(value) {
  if (value == "NaN" || value == "Inf" || value == "-Inf") {
    return 0; // TODO: what should we really do here?
  } else {
    return parseFloat(value)
  }
}

function transformData(json) {
  var palette = new Rickshaw.Color.Palette();
  if (json.Type != "matrix") {
    alert("Result is not of matrix type! Please enter a correct expression.");
    return [];
  }
  var data = json.Value.map(function(ts) {
    return {
      name: metricToTsName(ts.Metric),
      data: ts.Values.map(function(value) {
        return {
          x: value.Timestamp,
          y: parseValue(value.Value)
        }
      }),
      color: palette.color()
    };
  });
  Rickshaw.Series.zeroFill(data);
  return data;
}

function showGraph() {
  graph = new Rickshaw.Graph({
    element: document.querySelector("#chart"),
    height: 800,
    renderer: ($("#stacked").is(":checked") ? "stack" : "line"),
    interpolation: "linear",
    series: data
  });

  var x_axis = new Rickshaw.Graph.Axis.Time({ graph: graph });

  var y_axis = new Rickshaw.Graph.Axis.Y({
    graph: graph,
    tickFormat: Rickshaw.Fixtures.Number.formatKMBT,
  });

  graph.render();
}

function updateGraph(reloadGraph) {
  if (graph == null || reloadGraph) {
    $("#chart").empty();
    $("#legend").empty();
    showGraph();
  } else {
    graph.configure({
      renderer: ($("#stacked").is(":checked") ? "stack" : "line"),
      interpolation: "linear",
      series: data
    });
    graph.render();
  }

  var hoverDetail = new Rickshaw.Graph.HoverDetail({
    graph: graph
  });

  var legend = new Rickshaw.Graph.Legend({
    element: document.querySelector("#legend"),
    graph: graph
  });

  var shelving = new Rickshaw.Graph.Behavior.Series.Toggle({
    graph: graph,
    legend: legend
  });
}

function init() {
  jQuery.ajaxSetup({
    cache: false
  });
  $("#spinner").hide();
  $("#query_form").submit(submitQuery);
  $("#inc_range").click(increaseRange);
  $("#dec_range").click(decreaseRange);
  $("#stacked").change(updateGraph);
  $("#expr").focus();
}

$(init);
