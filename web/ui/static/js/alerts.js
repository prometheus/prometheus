function init() {
  $(".alert_header").click(function() {
    var expanderIcon = $(this).find("i.icon-chevron-down");
    if (expanderIcon.length !== 0) {
      expanderIcon.removeClass("icon-chevron-down").addClass("icon-chevron-up");
    } else {
      var collapserIcon = $(this).find("i.icon-chevron-up");
      collapserIcon.removeClass("icon-chevron-up").addClass("icon-chevron-down");
    }
    $(this).next().toggle();
  });

  $(".filters button.show-annotations").click(function(e) {
    const button = $(e.target);
    const icon = $(e.target).children("i");

    if (icon.hasClass("glyphicon-unchecked")) {
      icon.removeClass("glyphicon-unchecked")
          .addClass("glyphicon-check btn-primary");
      button.addClass("is-checked");

      $(".alert_annotations").show();
      $(".alert_annotations_header").show();
    } else if (icon.hasClass("glyphicon-check")) {
      icon.removeClass("glyphicon-check btn-primary")
          .addClass("glyphicon-unchecked");
      button.removeClass("is-checked");

      $(".alert_annotations").hide();
      $(".alert_annotations_header").hide();
    }
  });
}

$(init);
