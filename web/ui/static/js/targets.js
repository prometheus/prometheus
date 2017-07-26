function init() {
  var ls = localStorage;
  $(".job_header").click(function() {
    var selector = $(this).attr("data-target");
    ls.setItem(selector, !$(selector).hasClass("in"));
  });

  $(".job_header").each(function(i, obj) {
    var selector = $(obj).attr("data-target");
    if (ls.getItem(selector) === "true" || ls.getItem(selector) === null) {
      $(selector).addClass("in");
    }
  });
}

$(init);
