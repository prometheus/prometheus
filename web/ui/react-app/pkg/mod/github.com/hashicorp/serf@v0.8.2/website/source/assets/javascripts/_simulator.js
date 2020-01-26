//= require lib/_highcharts

var SerfSimulator = {
  // Defaults
  graphSettings: {
    chart: {
      type: 'spline'
    },
    title: {
      text: null
    },
    xAxis: {
      title: {
        text: "Time (sec)",
        style: {
          color: "#B03C44",
        },
      }
    },
    yAxis: {
      title: {
        text: 'Convergence %',
        style: {
          color: "#B03C44",
        },
      },
      min: 0.0,
      max: 100.0
    },
    tooltip: {
      formatter: function() {
        return '<strong>'+ (Math.round(this.y*1000.0)/1000.0) +'%</strong><br/>'
      }
    },
    legend: {
      enabled: false
    },
    series: [
      {
        name: 'Convergence Rate',
        data: [],
        color: "#B03C44",
      }
    ]
  },
  maxConverge: 0.9999,
  interval: 0.2,
  fanout : 3,
  nodes: 30,
  packetLoss: 0,
  nodeFail: 0,

  // init does all the stuff after the body is loaded
  init: function() {
    // Do not init if the graph is not present
    if (!$("#graph").length) {
      return
    }

    this.$graph = $("#graph");
    this.$graph.highcharts(this.graphSettings);
    this.$graphed = this.$graph.highcharts();

    this.$interval = $("#interval");
    this.$fanout   = $("#fanout");
    this.$nodes    = $("#nodes");
    this.$loss     = $("#packetloss");
    this.$failed   = $("#failed");
    this.$bytes    = $("#bytes");

    simulator = this;

    // Draw initial
    this.draw();

    this.$interval.on("change", function() {
      var interval = Number(this.value);
      if (isNaN(interval) || interval <= 0) {
        alert("Gossip interval must be a positive value!");
        return;
      }
      simulator.interval = interval;
      simulator.draw();
    });

    this.$fanout.on("change", function() {
      var fanout = Number(this.value);
      if (isNaN(fanout) || fanout <= 0) {
        alert("Gossip fanout must be a positive value!");
        return;
      }
      simulator.fanout = fanout;
      simulator.draw();
    });

    this.$nodes.on("change", function() {
      var nodes = Number(this.value);
      if (isNaN(nodes) || nodes <= 1) {
        alert("Must have at least one node!");
        return;
      }
      simulator.nodes = nodes;
      simulator.draw();
    });

    this.$loss.on("change", function() {
      var pkt = Number(this.value);
      if (isNaN(pkt) || pkt < 0 || pkt >= 100) {
        alert("Packet loss must be greater or equal to 0 and less than 100");
        return;
      }
      simulator.packetLoss = (pkt / 100.0);
      simulator.draw();
    });

    this.$failed.on("change", function() {
      var failed = Number(this.value);
      if (isNaN(failed) || failed < 0 || failed >= 100) {
        alert("Failure rate must be greater or equal to 0 and less than 100");
        return;
      }
      simulator.nodeFail = (failed / 100.0);
      simulator.draw();
    });
  },

  draw: function() {
    var data = this.seriesData();
    this.$graphed.series[0].setData(data, false)/
    this.$graphed.redraw();

    var kilobits = this.bytesUsed() * 8;
    var used = Math.round((kilobits / 1024) * 10) / 10;
    this.$bytes.html("" + used);
  },

  convergenceAtRound: function(x) {
    var contact = (this.fanout / this.nodes) * (1 - this.packetLoss) * (1 - this.nodeFail) * 0.5
    var numInf = this.nodes / (1 + (this.nodes+1) * Math.pow(Math.E, -1*contact*this.nodes*x))
    return numInf / this.nodes
  },

  roundLength: function() {
    return this.interval
  },

  seriesData: function() {
    var data = []
    var lastVal = 0
    var round = 0
    var roundLength = this.roundLength()
    while (lastVal < this.maxConverge && round < 100) {
      lastVal = this.convergenceAtRound(round)
      data.push([round * roundLength, lastVal*100.0])
      round++
    }
    return data
  },

  bytesUsed: function() {
    var roundLength = this.roundLength()
    var roundsPerSec = 1 / roundLength
    var packetSize = 1400
    var send = packetSize * this.fanout * roundsPerSec
    return send * 2
  },
}

// Handle document ready function and the turbolinks load.
$(document).on("turbolinks:load", function() {
  SerfSimulator.init()
});
