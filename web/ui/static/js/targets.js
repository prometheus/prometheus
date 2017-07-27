function init() {
  $(".job_header").click(function() {
    var selector = $(this).attr("data-target");
    window.localStorage.setItem(selector, !$(selector).hasClass("in"));
  });

  $(".job_header").each(function(i, obj) {
    var selector = $(obj).attr("data-target");
    if (window.localStorage.getItem(selector) === "true" || window.localStorage.getItem(selector) === null) {
        $(selector).addClass("in");
    }
  });
}

$(init);
