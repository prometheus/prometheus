function init() {
  $(".alert_header").click(function() {
    var expanderIcon = $(this).find("i.icon-chevron-down");
    if (expanderIcon.length != 0) {
      expanderIcon.removeClass("icon-chevron-down").addClass("icon-chevron-up");
    } else {
      var collapserIcon = $(this).find("i.icon-chevron-up");
      collapserIcon.removeClass("icon-chevron-up").addClass("icon-chevron-down");
    }
    $(this).next().toggle();
  });

  $(".silence_alert_link, .silence_children_link").click(function() {
    alert("Silencing is not yet supported.");
  });
}

$(init);
