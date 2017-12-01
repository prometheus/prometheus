function toggleJobTable(button, shouldExpand){
  if (button.length === 0) { return; }

  if (shouldExpand) {
    button.removeClass("collapsed-table").addClass("expanded-table").html("show less");
  } else {
    button.removeClass("expanded-table").addClass("collapsed-table").html("show more");
  }

  button.parents(".table-container").find("table").toggle(shouldExpand);
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

  $(".filters button.unhealthy-targets").click(function(e) {
    const button = $(e.target);
    const icon = $(e.target).children("i");

    if (icon.hasClass("glyphicon-unchecked")) {
      icon.removeClass("glyphicon-unchecked")
          .addClass("glyphicon-check btn-primary");
      button.addClass("is-checked");

      $(".table-container").each(showUnhealthy);
    } else if (icon.hasClass("glyphicon-check")) {
      icon.removeClass("glyphicon-check btn-primary")
          .addClass("glyphicon-unchecked");
      button.removeClass("is-checked");

      $(".table-container").each(showAll);
    }
  });
}

function showAll(_, container) {
  $(container).show();
}

function showUnhealthy(_, container) {
  const isHealthy = $(container).find("h2").attr("class").indexOf("danger") < 0;
  if (isHealthy) { $(container).hide(); }
}

$(init);
