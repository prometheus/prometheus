var url = "http://juliusv.com:9090/api/query?expr=targets_healthy_scrape_latency_ms%5B'10m'%5D&json=JSON";

// Graph options
 // Grid off/on
 // Stacked off/on
 // Area off/on
 // Legend position
 // Short link
// Graph title
// Palette
// Background
// Enable tooltips
// width/height
// Axis options
 // Y-Range min/max
 // (X-Range min/max)
 // X-Axis format
 // Y-Axis format
 // Y-Axis title
 // X-Axis title
 // Log scale

var graph = null;
var data = [];

function submitQuery() {
  $("#spinner").show();
  $("#load_time").empty();
  var form = $("#queryForm");
  var startTime = new Date().getTime();

  $.ajax({
        method: form.attr("method"),
        url: form.attr("action"),
        dataType: "json",
        data: form.serialize(),
        success: function(json, textStatus) {
                data = transformData(json);
                if (data.length == 0) {
                        alert("No datapoints found.");
                        return;
                }
                graph = null;
                $("#chart").empty();
                $("#legend").empty();
                $("#y_axis").empty();
                showGraph();
        },
        error: function() {
                alert("Error executing query!");
        },
        complete: function() {
                var duration = new Date().getTime() - startTime;
                $("#load_time").html("Load time: " + duration + "ms");
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
        if (value == "NaN") {
                return 0; // TODO: what to do here?
        } else {
                return parseFloat(value)
        }
}

function transformData(json) {
        var palette = new Rickshaw.Color.Palette();
        if (json.Type != "matrix") {
                alert("Result is not of matrix type!");
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
                var metricStr = ts['name'];
        });
        return data;
}

function showGraph() {
    graph = new Rickshaw.Graph( {
            element: document.querySelector("#chart"),
            width: 1200,
            height: 800,
            renderer: 'line',
            series: data
    } );
    //graph.configure({offset: 'wiggle'});

    var x_axis = new Rickshaw.Graph.Axis.Time( { graph: graph } );

    var y_axis = new Rickshaw.Graph.Axis.Y( {
            element: document.querySelector("#y_axis"),
            graph: graph,
            orientation: 'left',
            tickFormat: Rickshaw.Fixtures.Number.formatKMBT,
    } );

    var legend = new Rickshaw.Graph.Legend( {
            element: document.querySelector('#legend'),
            graph: graph
    } );

    graph.render();

    var hoverDetail = new Rickshaw.Graph.HoverDetail( {
            graph: graph
    } );

    var shelving = new Rickshaw.Graph.Behavior.Series.Toggle( {
            graph: graph,
            legend: legend
    } );
}

function init() {
  jQuery.ajaxSetup({
      cache: false
  });
  $("#spinner").hide();
  $("#queryForm").submit(submitQuery);
  $("#expr").focus();
}

$(init);
