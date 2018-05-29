var graphTemplate;
var alertStateToRowClass;
var alertStateToName;

var SECOND = 1000;

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

function mustacheFormatMap(map) {
  formatted = []
  for (var key in map) {
    formatted.push({
      'key': key,
      'value': map[key]
    })
  }
  return formatted
}


function reinitJQueryFunctions() {
  $(".alert_header").off();
  $("div.show-annotations").off();
  $("div.show-graphs").off();

  $(".alert_header").click(function() {
    $(this).next().toggle();
  });

  $("div.show-annotations").click(function() {
    const targetEl = $('div.show-annotations');
    const icon = $(targetEl).children('i');
    if (icon.hasClass('glyphicon-unchecked')) {
      $(".alert_annotations").show();
      $(".alert_annotations_header").show();
      $(targetEl).children('i').removeClass('glyphicon-unchecked').addClass('glyphicon-check');
      targetEl.addClass('is-checked');
    } else if (icon.hasClass('glyphicon-check')) {
      $(".alert_annotations").hide();
      $(".alert_annotations_header").hide();
      $(targetEl).children('i').removeClass('glyphicon-check').addClass('glyphicon-unchecked');
      targetEl.removeClass('is-checked');
    }
  });

  $("div.show-graphs").click(function() {
    const targetEl = $('div.show-graphs');
    const icon = $(targetEl).children('i');
    if (icon.hasClass('glyphicon-unchecked')) {
      $(".graph_header").show();
      $(".graph_body").show();
      $(targetEl).children('i').removeClass('glyphicon-unchecked').addClass('glyphicon-check');
      targetEl.addClass('is-checked');
    } else if (icon.hasClass('glyphicon-check')) {
      $(".graph_header").hide();
      $(".graph_body").hide();
      $(targetEl).children('i').removeClass('glyphicon-check').addClass('glyphicon-unchecked');
      targetEl.removeClass('is-checked');
    }
  });

}

/**
 * Graph
*/
var Graph = function(element, options, json) {
  this.el = element;
  this.graphHTML = null;
  this.options = options;
  this.rickshawGraph = null;
  this.data = [];
  this.json = json

  this.initialize();
};

Graph.timeFactors = {
  "y": 60 * 60 * 24 * 365,
  "w": 60 * 60 * 24 * 7,
  "d": 60 * 60 * 24,
  "h": 60 * 60,
  "m": 60,
  "s": 1
};

Graph.stepValues = [
  "1s", "10s", "1m", "5m", "15m", "30m", "1h", "2h", "6h", "12h", "1d", "2d",
  "1w", "2w", "4w", "8w", "1y", "2y"
];

Graph.numGraphs = 0;

Graph.prototype.initialize = function() {
  var self = this;
  self.id = Graph.numGraphs++;

  // Set default options.
  self.options.id = self.id;
  self.options.range_input = self.options.range_input || "1h";
  if (self.options.tab === undefined) {
    self.options.tab = 1;
  }

  // Draw graph controls and container from Handlebars template.

  var options = {
    'pathPrefix': PATH_PREFIX,
    'buildVersion': BUILD_VERSION,
    'ruleName': self.json.name,
    'activeAlerts': self.json.alerts,
    'htmlSnippet': self.json.htmlSnippet,
  };
  if(self.json.alerts) {
    options.activeAlertsLength = self.json.alerts.length;
  } else {
    options.activeAlertsLength = 0
  }
  var maxState = 0;
  for(i in options.activeAlerts) {
    options.activeAlerts[i].Labels = mustacheFormatMap(options.activeAlerts[i].Labels);
    options.activeAlerts[i].Annotations = mustacheFormatMap(options.activeAlerts[i].Annotations);
    options.activeAlerts[i].stateName = alertStateToName[options.activeAlerts[i].State];
    options.activeAlerts[i].stateClass = alertStateToRowClass[options.activeAlerts[i].State];
    options.activeAlerts[i].ActiveAt = (new Date(options.activeAlerts[i].ActiveAt)).toUTCString();
    if (options.activeAlerts[i].State > maxState) {
      maxState = options.activeAlerts[i].State;
    }
  }
  options.maxState = alertStateToRowClass[maxState]
  
  jQuery.extend(options, self.options);
  self.graphHTML = $(Mustache.render(graphTemplate, options));
  self.el.append(self.graphHTML);
  reinitJQueryFunctions();

  // Get references to all the interesting elements in the graph container and
  // bind event handlers.
  var graphWrapper = self.el.find("#graph_wrapper" + self.id);
  self.queryForm = graphWrapper.find(".query_form");

  self.rangeInput = self.queryForm.find("input[name=range_input]");
  self.stackedBtn = self.queryForm.find(".stacked_btn");
  self.stacked = self.queryForm.find("input[name=stacked]");
  self.refreshInterval = self.queryForm.find("select[name=refresh]");

  self.error = graphWrapper.find(".error").hide();
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
  self.endDate.on("dp.change", function() { self.initGraphUpdate(); });

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

  self.spinner.hide();

  self.initGraphUpdate();

};

Graph.prototype.parseDuration = function(rangeText) {
  var rangeRE = new RegExp("^([0-9]+)([ywdhms]+)$");
  var matches = rangeText.match(rangeRE);
  if (!matches) { return; }
  if (matches.length != 3) {
    return 60;
  }
  var value = parseInt(matches[1]);
  var unit = matches[2];
  return value * Graph.timeFactors[unit];
};

Graph.prototype.getEndDate = function() {
  var self = this;
  if (!self.endDate || !self.endDate.val()) {
    return moment();
  }
  return self.endDate.data('DateTimePicker').date();
};

Graph.prototype.initGraphUpdate = function() {
  var self = this;
  self.clearError();

  var rangeSeconds = self.parseDuration(self.rangeInput.val());
  var resolution = parseInt(self.queryForm.find("input[name=step_input]").val()) || Math.max(Math.floor(rangeSeconds / 250), 1);
  var endDate = self.getEndDate() / 1000;
  var params = {};
  params.start = endDate - rangeSeconds;
  params.end = endDate;
  params.step = resolution;
  self.params = params;

  self.handleGraphResponse(self.json.matrixResult);
};

Graph.prototype.showError = function(msg) {
  var self = this;
  self.error.text(msg);
  self.error.show();
};

Graph.prototype.clearError = function(msg) {
  var self = this;
  self.error.text('');
  self.error.hide();
};

Graph.prototype.renderLabels = function(labels) {
  var labelStrings = [];
  for (var label in labels) {
    if (label != "__name__") {
      labelStrings.push("<strong>" + label + "</strong>: " + escapeHTML(labels[label]));
    }
  }
  return labels = "<div class=\"labels\">" + labelStrings.join("<br>") + "</div>";
};

Graph.prototype.metricToTsName = function(labels) {
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

Graph.prototype.parseValue = function(value) {
  var val = parseFloat(value);
  if (isNaN(val)) {
    // "+Inf", "-Inf", "+Inf" will be parsed into NaN by parseFloat(). The
    // can't be graphed, so show them as gaps (null).
    return null;
  }
  return val;
};

Graph.prototype.transformData = function(json) {
  var self = this;
  var palette = new Rickshaw.Color.Palette();
  if (json.resultType != "matrix") {
    self.showError("Result is not of matrix type! Please enter a correct expression.");
    return [];
  }
  json.result = json.result || []
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
    var temp = ts.values.map(function(value) {
      return {
        x: value[0],
        y: self.parseValue(value[1])
      };
    });
    return {
      name: name,
      labels: labels,
      data: temp,
      tempData: temp, // Explained in 'updateGraph'.
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

Graph.prototype.updateGraph = function() {
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

  // For some reason 'data' field was getting restricted to size of 258-259 (observed with console log).
  // But, you could get original array by accessing .data field manually (but graph did not do it).
  // Hence we this to restore the original array from 'tempData' to 'data' field.
  // self.data = self.data.map(function(val){
  //   val.data = val.tempData;
  //   return val;
  // });

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
    if (min === max) {
      self.rickshawGraph.max = max + 1;
      self.rickshawGraph.min = min - 1;
    } else {
      self.rickshawGraph.max = max + (0.1*(Math.abs(max - min)));
      self.rickshawGraph.min = min - (0.1*(Math.abs(max - min)));
    }
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

  // self.handleChange();
};

Graph.prototype.resizeGraph = function() {
  var self = this;
  if (self.rickshawGraph !== null) {
    self.rickshawGraph.configure({
      width: Math.max(self.graph.innerWidth() - 80, 200),
    });
    self.rickshawGraph.render();
  }
};

Graph.prototype.handleGraphResponse = function(json) {
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
  self.updateGraph();
};

Graph.prototype.formatKMBT = function(y) {
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

function greenHtml(text) {
  return '<div style="color:green;">' + text + '</div>';
}

function redHtml(text) {
  return '<div style="color:red;">' + text + '</div>';
}

var replaceRules = function(json) {
  $("#graph_container").empty();
  Graph.numGraphs = 0;

  for(i in json.data.ruleResults) {
    var graph = new Graph(
      $("#graph_container"),
      {},
      json.data.ruleResults[i]
    );
    $(window).resize(function() {
      graph.resizeGraph();
    });
  }

};

function initRuleTesting() {
  $("#ruleTestExecute").click(function() {
    var text = ace.edit("ruleTextArea").getValue();
    $("#ruleTestInfo").html("Testing...");          

    $.ajax({
      method: 'POST',
      url: PATH_PREFIX + "/api/v1/alerts_testing",
      dataType: "json",
      data: JSON.stringify({
        RuleText: encodeURIComponent(text)
      }),
      success: function(json) {
        var data = json.data
        if(data.isError) {
          var errStr = "Error message:<br/>"
          var len = data.errors.length 
          for(var i = 0; i<len; i++) {
            errStr += "(" + (i+1) + ") " + data.errors[i] + '<br/>'
          }
          $("#ruleTestInfo").html(redHtml(errStr));
        } else {
          $("#ruleTestInfo").html(greenHtml(data.success));  

          alertStateToRowClass = json.data.alertStateToRowClass;
          alertStateToName = json.data.alertStateToName;
        }
        replaceRules(json);        
      },
      error: function(jqXHR, textStatus, errorThrown) {
        $("#ruleTestInfo").html(redHtml("ERROR: "+errorThrown));
      }
    });

  });
  
}

function initEditor() {
  $("#ruleTextArea").html("# Enter your entire alert rule file here");
  var e = ace.edit("ruleTextArea");
  e.setTheme("ace/theme/xcode");
  e.getSession().setMode("ace/mode/yaml");
  e.setFontSize("12pt");
  e.focus();
}

function init() {
  $.ajaxSetup({
    cache: false
  });

  $("#showAll").click(function() {
    $(".alert_details").show();
  });

  $("#hideAll").click(function() {
    $(".alert_details").hide();
  });

  $.ajax({
    url: PATH_PREFIX + "/static/js/alert_testing/graph_template.handlebar?v=" + BUILD_VERSION,
    success: function(data) {
      graphTemplate = data;
      Mustache.parse(data);
      initRuleTesting();
      initEditor();
    }
  });
}

$(init);
