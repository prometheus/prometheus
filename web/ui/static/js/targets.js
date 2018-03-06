function toggleJobTable(button, shouldExpand){
  if (button.length === 0) { return; }

  if (shouldExpand) {
    button.removeClass("collapsed-table").addClass("expanded-table").html("show less");
  } else {
    button.removeClass("expanded-table").addClass("collapsed-table").html("show more");
  }

  button.parents(".table-container").find("table").toggle(shouldExpand);
}

function showAll(_, container) {
  $(container).show();
}

function showUnhealthy(_, container) {
  const isHealthy = $(container).find("h2").attr("class").indexOf("danger") < 0;
  if (isHealthy) { $(container).hide(); }
}

function showHealthy(_, container) {
  const isUnhealthy = $(container).find("h2").attr("class").indexOf("danger") > 0;
    if (isUnhealthy) { $(container).hide(); }
}

function init() {
  $("button.targets").click(function () {
    const tableTitle = $(this).closest("h2").find("a").attr("id");

    if ($(this).hasClass("collapsed-table")) {
      localStorage.setItem(tableTitle, "expanded");
      toggleJobTable($(this), true);
    } else if ($(this).hasClass("expanded-table")) {
      localStorage.setItem(tableTitle, "collapsed");
      toggleJobTable($(this), false);
    }
  });

  $(".job_header a").each(function (_, link) {
    const cachedTableState = localStorage.getItem($(link).attr("id"));
    if (cachedTableState === "collapsed") {
      toggleJobTable($(this).siblings("button"), false);
    }
  });

  $(".filters button.unhealthy-targets, .filters button.healthy-targets").click(function(e) {
    const button = $(e.target);
    const icon = $(e.target).children("i");

    if (icon.hasClass("glyphicon-unchecked")) {
      icon.removeClass("glyphicon-unchecked")
          .addClass("glyphicon-check btn-primary");
      button.addClass("is-checked");

      if (button.hasClass("unhealthy-targets")) {
        $(".table-container").each(showUnhealthy);
        $(".filters button.healthy-targets").prop("disabled", true);
      } else if (button.hasClass("healthy-targets")) {
        $(".table-container").each(showHealthy);
        $(".filters button.unhealthy-targets").prop("disabled", true);
      }
    } else if (icon.hasClass("glyphicon-check")) {
      if (button.hasClass("unhealthy-targets")) {
        $(".filters button.healthy-targets").prop("disabled", false);
      } else if (button.hasClass("healthy-targets")) {
        $(".filters button.unhealthy-targets").prop("disabled", false);
      }
      icon.removeClass("glyphicon-check btn-primary")
          .addClass("glyphicon-unchecked");
      button.removeClass("is-checked");

      $(".table-container").each(showAll);
    }
  });
}

$(init);
