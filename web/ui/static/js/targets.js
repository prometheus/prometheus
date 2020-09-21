function toggleJobTable(button, shouldExpand){
  if (button.length === 0) { return; }

  if (shouldExpand) {
    button.removeClass("collapsed-table").addClass("expanded-table").html("show less");
  } else {
    button.removeClass("expanded-table").addClass("collapsed-table").html("show more");
  }

  button.parents(".table-container").find("table").toggle(shouldExpand);
  button.parents(".table-container").find(".collapsed-element").toggle(shouldExpand);
}

function showAll(_, container) {
  $(container).show();
}

function showUnhealthy(_, container) {
  const isHealthy = $(container).find("h2").attr("class").indexOf("danger") < 0;
  if (isHealthy) { $(container).hide(); }
}

var allCollapsed = false;

function init() {
  if ($("#unhealthy-targets").length) {
    if (!localStorage.selectedTargetsTab || localStorage.selectedTargetsTab == "all-targets") {
      $("#all-targets").parent().addClass("active");
      $(".table-container").each(showAll);
    } else if (localStorage.selectedTargetsTab == "unhealthy-targets") {
      $("#unhealthy-targets").parent().addClass("active");
      $(".table-container").each(showUnhealthy);
    }
  } else {
    $(".table-container").each(showAll);
  }

  $("button.targets").click(function() {
    const tableTitle = $(this).closest("h2").find("a").attr("id");

    if ($(this).hasClass("collapsed-table")) {
      localStorage.setItem(tableTitle, "expanded");
      toggleJobTable($(this), true);
    } else if ($(this).hasClass("expanded-table")) {
      localStorage.setItem(tableTitle, "collapsed");
      toggleJobTable($(this), false);
    }
  });

  $(".collapse-all").click(function() {

    // locally store state of allCollapsed
    previousAllCollapsed = allCollapsed;

    // conditionally change the text of the button
    if (allCollapsed == false) {
      $(this).html("Expand All");
      allCollapsed = true;
    } else {
      $(this).html("Collapse All");
      allCollapsed = false;
    }

    $("button.targets").each(function(_, thisButton) {
      const tableTitle = $(thisButton).closest("h2").find("a").attr("id");

      if (previousAllCollapsed == false) {

        // collapse all the jobs
        if ($(this).hasClass("expanded-table")) {
          localStorage.setItem(tableTitle, "collapsed");
          toggleJobTable($(thisButton), false);
        }
      } else {

        // expand all the jobs
        if ($(this).hasClass("collapsed-table")) {
          localStorage.setItem(tableTitle, "expanded");
          toggleJobTable($(this), true);
        }
      }
    });
  });

  $(".job_header a").each(function (_, link) {
    const cachedTableState = localStorage.getItem($(link).attr("id"));
    if (cachedTableState === "collapsed") {
      toggleJobTable($(this).siblings("button"), false);
    }
  });

  $("#showTargets :input").change(function() {
    const target = $(this).attr("id");

    if (target === "all-targets") {
      $(".table-container").each(showAll);
      localStorage.setItem("selectedTargetsTab", "all-targets");
    } else if (target === "unhealthy-targets") {
      $(".table-container").each(showUnhealthy);
      localStorage.setItem("selectedTargetsTab", "unhealthy-targets");
    }
  });
}

$(init);
